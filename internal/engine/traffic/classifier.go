package traffic

import (
	"fmt"
	"net/netip"
	"sort"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

// FlowClassifier groups LocatedFlows into FlowEquivalenceClasses
// based on identical forwarding behavior.
type FlowClassifier struct {
	universe model.PrefixUniverse
}

// NewFlowClassifier creates a new FlowClassifier.
func NewFlowClassifier(universe model.PrefixUniverse) *FlowClassifier {
	return &FlowClassifier{universe: universe}
}

// Classify groups a set of located flows into equivalence classes.
// Flows are grouped by (PrefixClassID + Protocol + DstPort + IngressNode).
func (c *FlowClassifier) Classify(flows []model.LocatedFlow) []model.FlowEquivalenceClass {
	groups := map[string]*model.FlowEquivalenceClass{}
	keys := make([]string, 0) // maintain insertion order for determinism
	for _, f := range flows {
		dstPrefix := model.PrefixFromNetIP(netip.PrefixFrom(f.Flow.DstIP, f.Flow.DstIP.BitLen()))
		classID, _ := c.universe.ClassForPrefix(dstPrefix)
		key := fmt.Sprintf("%d|%s|%d|%s", classID, f.Flow.Protocol, f.Flow.DstPort, f.IngressNode)
		if _, ok := groups[key]; !ok {
			groups[key] = &model.FlowEquivalenceClass{
				ID:          len(groups),
				IngressNode: f.IngressNode,
				IngressIntf: f.IngressIntf,
			}
			keys = append(keys, key)
		}
		groups[key].TotalBytes += f.Bytes
		groups[key].FlowCount++
	}

	// Convert map to slice in insertion order
	result := make([]model.FlowEquivalenceClass, 0, len(groups))
	for _, key := range keys {
		result = append(result, *groups[key])
	}

	// Assign sequential IDs based on final order
	sort.Slice(result, func(i, j int) bool {
		if result[i].IngressNode != result[j].IngressNode {
			return result[i].IngressNode < result[j].IngressNode
		}
		return result[i].ID < result[j].ID
	})
	for i := range result {
		result[i].ID = i
	}

	return result
}
