package traffic

import (
	"fmt"

	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/engine/dataplane"
)

// TrafficSimulator simulates traffic flows on a network topology
// and computes per-link load.
type TrafficSimulator struct {
	eng *dataplane.Engine
	idx *model.TopologyIndex
}

// NewSimulator creates a new TrafficSimulator.
func NewSimulator(eng *dataplane.Engine, idx *model.TopologyIndex) *TrafficSimulator {
	return &TrafficSimulator{eng: eng, idx: idx}
}

// Simulate computes link loads for the given flow equivalence classes.
func (ts *TrafficSimulator) Simulate(ecs []model.FlowEquivalenceClass) (model.TrafficResult, error) {
	linkBytes := map[string]uint64{}
	for _, ec := range ecs {
		if ec.TotalBytes == 0 {
			continue
		}
		if err := ts.simulateEC(linkBytes, ec); err != nil {
			return model.TrafficResult{}, err
		}
	}
	return model.TrafficResult{
		LinkLoads: ts.buildLinkLoads(linkBytes),
	}, nil
}

func (ts *TrafficSimulator) simulateEC(linkBytes map[string]uint64, ec model.FlowEquivalenceClass) error {
	dstIP := firstAddrFromPrefixSet(ec.PacketClass.DstSet)
	if dstIP == "" {
		return fmt.Errorf("cannot resolve destination IP from FEC packet class")
	}

	// Build packet spec from the FEC's packet class
	spec := ec.PacketClass.Spec()

	// Look up FIB on the ingress node to check for ECMP
	fibEntries := ts.eng.SymbolicLookupFIB(ec.IngressNode, dstIP)
	if len(fibEntries) == 0 {
		return nil // no route, skip
	}

	// Check if any of the matching entries have ECMP next-hops
	var ecmpNhs []dataplane.NextHopEntry
	for _, candidate := range fibEntries {
		if len(candidate.Entry.NextHops) > 0 {
			ecmpNhs = candidate.Entry.NextHops
			break
		}
	}

	if len(ecmpNhs) > 1 {
		// ECMP: distribute bytes across ALL links in each ECMP path
		for _, nh := range ecmpNhs {
			// Find the ingress→next-hop link
			firstLink, ok := ts.idx.LinkBetween(ec.IngressNode, nh.Node)
			if !ok {
				continue
			}
			// Resolve full path from the next-hop node to the destination
			path, ok, _ := ts.eng.PacketReachableSpec(nh.Node, dstIP, spec, failure.Set{})
			if !ok {
				// Fall back to just the first-hop link
				linkBytes[firstLink.Name] += uint64(float64(ec.TotalBytes) * nh.Weight)
				continue
			}
			// Prepend the first-hop link and distribute weighted bytes
			weightedBytes := uint64(float64(ec.TotalBytes) * nh.Weight)
			linkBytes[firstLink.Name] += weightedBytes
			for _, link := range path.Links {
				linkBytes[link] += weightedBytes
			}
		}
	} else {
		// Single path: resolve full path and add bytes to all links
		path, ok, _ := ts.eng.PacketReachableSpec(ec.IngressNode, dstIP, spec, failure.Set{})
		if !ok {
			return nil // unreachable, skip
		}
		for _, link := range path.Links {
			linkBytes[link] += ec.TotalBytes
		}
	}

	return nil
}

func (ts *TrafficSimulator) buildLinkLoads(linkBytes map[string]uint64) map[string]model.LinkLoad {
	loads := map[string]model.LinkLoad{}
	for name, bytes := range linkBytes {
		ll := model.LinkLoad{
			LinkName: name,
			Bytes:    bytes,
		}
		if ts.idx != nil {
			if link, ok := ts.idx.Link(name); ok {
				ll.Capacity = uint64(linkBandwidth(link))
			}
		}
		loads[name] = ll
	}
	return loads
}

// firstAddrFromPrefixSet extracts the first IPv4 address from a PrefixSet.
func firstAddrFromPrefixSet(set model.PrefixSet) string {
	if set == nil {
		return ""
	}
	switch s := set.(type) {
	case model.ExactPrefixSet:
		addr := s.Prefix.Addr()
		if addr.IsValid() {
			return addr.String()
		}
	case model.UnionPrefixSet:
		if len(s.Sets) > 0 {
			return firstAddrFromPrefixSet(s.Sets[0])
		}
	case model.AnyPrefixSet:
		return "0.0.0.0"
	}
	return ""
}

// linkBandwidth returns the link bandwidth in bps from a Link's role.
func linkBandwidth(l model.Link) int {
	switch l.Role {
	case "core":
		return 10_000_000_000 // 10 Gbps
	case "edge":
		return 1_000_000_000 // 1 Gbps
	default:
		return 1_000_000_000 // default 1 Gbps
	}
}
