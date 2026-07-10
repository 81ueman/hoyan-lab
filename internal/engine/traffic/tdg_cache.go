package traffic

import (
	"fmt"

	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

// cachedTDG stores a built TDG along with its key metadata.
type cachedTDG struct {
	tdg     *model.TDG
	ingress string
	classID int
}

// TDGCache caches TDGs keyed by ingress node and packet class.
// When a failure is specified, the cached TDG is cloned and edges for
// failed links/nodes are pruned.
type TDGCache struct {
	entries map[string]*cachedTDG
}

// NewTDGCache creates an empty TDG cache.
func NewTDGCache() *TDGCache {
	return &TDGCache{
		entries: make(map[string]*cachedTDG),
	}
}

// cacheKey returns the cache key for an ingress node and packet class.
func cacheKey(ingress string, classID int) string {
	return fmt.Sprintf("%s|%d", ingress, classID)
}

// GetOrBuild returns a TDG for the given ingress and packet class.
// If a cached TDG exists, it is returned. Otherwise, a new TDG is built
// using BuildTDG and cached.
func (c *TDGCache) GetOrBuild(ingress string, pc model.PacketClass, fibs FIBTable) *model.TDG {
	key := cacheKey(ingress, int(pc.PrefixClassID))
	if entry, ok := c.entries[key]; ok {
		return entry.tdg
	}

	tdg := BuildTDG(ingress, pc, fibs)
	c.entries[key] = &cachedTDG{
		tdg:     tdg,
		ingress: ingress,
		classID: int(pc.PrefixClassID),
	}
	return tdg
}

// ApplyFailure creates a shallow clone of the TDG but removes edges whose
// link or endpoint node is in the failure set. For affected ECMP groups,
// weights are redistributed among remaining members.
func (c *TDGCache) ApplyFailure(tdg *model.TDG, failures failure.Set) *model.TDG {
	// Build set of failed node names for fast lookup
	failedNodes := make(map[string]bool)
	for nodeID := range failures.Nodes {
		failedNodes[string(nodeID)] = true
	}

	// Build set of failed link names for fast lookup
	failedLinks := make(map[string]bool)
	for linkID := range failures.Links {
		failedLinks[string(linkID)] = true
	}

	// Clone the TDG (shallow copy)
	cloned := cloneTDG(tdg)
	if cloned == nil {
		return nil
	}

	// Collect edges to remove (can't remove during iteration)
	type edgeKey struct{ from, to string }
	var toRemove []edgeKey

	for _, edge := range cloned.Edges {
		fromNode := edge.From.Node
		toNode := edge.To.Node
		linkName := linkName(fromNode, toNode)

		if failedLinks[linkName] || failedNodes[fromNode] || failedNodes[toNode] {
			toRemove = append(toRemove, edgeKey{fromNode, toNode})
		}
	}

	// Group removed edges by source node for ECMP rebalancing
	removedByNode := make(map[string]int)
	for _, ek := range toRemove {
		removedByNode[ek.from]++
	}

	// Remove affected edges
	for _, ek := range toRemove {
		cloned.RemoveEdge(ek.from, ek.to)
	}

	// Rebalance ECMP weights for nodes that lost some but not all out-edges
	for fromNode := range removedByNode {
		outEdges := cloned.OutEdges(fromNode)
		if len(outEdges) == 0 {
			continue
		}
		// Only rebalance if at least one edge was removed from this source
		remainingWeight := 0.0
		for _, edge := range outEdges {
			remainingWeight += edge.Weight
		}
		if remainingWeight > 0 && remainingWeight < 1.0 {
			for _, edge := range outEdges {
				newWeight := edge.Weight / remainingWeight
				cloned.SetEdgeWeight(edge.From.Node, edge.To.Node, newWeight)
			}
		}
	}

	return cloned
}

// cloneTDG creates a shallow copy of a TDG. Nodes are shared (immutable metadata),
// but edges are deep-copied so mutations don't affect the original.
func cloneTDG(tdg *model.TDG) *model.TDG {
	cloned := model.NewTDG()

	// Copy all nodes
	for _, node := range tdg.Nodes {
		cloned.AddNode(node.Node, node.VRF, node.Stage, node.PacketClassID)
	}

	// Deep copy edges
	for _, edge := range tdg.Edges {
		cloned.AddEdge(edge.From.Node, edge.To.Node, edge.Weight)
	}

	// Copy root
	if tdg.Root != nil {
		if err := cloned.SetRoot(tdg.Root.Node); err != nil {
			return nil
		}
	}

	// Copy sinks
	for _, sink := range tdg.Sinks {
		cloned.AddSink(sink.Node)
	}

	return cloned
}
