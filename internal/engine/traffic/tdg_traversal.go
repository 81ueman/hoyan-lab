package traffic

import (
	"fmt"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

// linkName generates a link key from node names.
func linkName(from, to string) string {
	return fmt.Sprintf("%s->%s", from, to)
}

// Traverse processes traffic through the TDG, accumulating link loads.
// It returns a map of link name -> bytes carried.
func Traverse(tdg *model.TDG, totalBytes uint64) map[string]uint64 {
	linkBytes := map[string]uint64{}
	nodeLoads := map[string]uint64{}

	if tdg.Root == nil || totalBytes == 0 {
		return linkBytes
	}

	nodeLoads[tdg.Root.Node] = totalBytes

	// Topological order traversal (BFS from root)
	for _, node := range tdg.TopologicalOrder() {
		bytes := nodeLoads[node.Node]
		if bytes == 0 {
			continue
		}

		outEdges := tdg.OutEdges(node.Node)
		if len(outEdges) == 0 {
			continue
		}

		for _, edge := range outEdges {
			edgeBytes := uint64(float64(bytes) * edge.Weight)
			if edgeBytes == 0 {
				continue
			}

			// Record link load
			name := linkName(edge.From.Node, edge.To.Node)
			linkBytes[name] += edgeBytes

			// Propagate to next node
			nodeLoads[edge.To.Node] += edgeBytes
		}
	}

	return linkBytes
}
