package traffic

import (
	"fmt"
	"net/netip"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

// lookupFIB finds the best matching FIB entry for a given prefix.
func lookupFIB(node string, dst netip.Addr, fibs FIBTable) *TrafficFIBEntry {
	entries, ok := fibs[node]
	if !ok {
		return nil
	}
	var best *TrafficFIBEntry
	bestBits := -1
	for i := range entries {
		e := &entries[i]
		if e.Prefix.Contains(dst) && e.Prefix.Bits() > bestBits {
			best = e
			bestBits = e.Prefix.Bits()
		}
	}
	return best
}

// BuildTDG constructs a Traffic Distribution Graph for a given root node,
// packet class, and FIB table. It returns the TDG with nodes and edges
// representing how traffic flows through the network.
func BuildTDG(rootNode string, packetClass model.PacketClass, fibs FIBTable) *model.TDG {
	tdg := model.NewTDG()

	// Determine destination address from packet class
	var dstAddr netip.Addr
	if packetClass.DstSet != nil {
		// Pick a representative address from the destination set
		dstAddr = representativeAddr(packetClass.DstSet)
	}

	// Create root node (ingress)
	tdg.AddNode(rootNode, "default", "ingress_acl", PrefixClassIDFromPacketClass(packetClass))
	if err := tdg.SetRoot(rootNode); err != nil {
		panic(fmt.Sprintf("BuildTDG: SetRoot(%s): %v", rootNode, err))
	}

	if !dstAddr.IsValid() {
		// No destination, just return with root node
		return tdg
	}

	visited := map[string]bool{}
	queue := []string{rootNode}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if visited[current] {
			continue
		}
		visited[current] = true

		// Look up FIB for this node + destination
		entry := lookupFIB(current, dstAddr, fibs)
		if entry == nil {
			// No FIB match, this is a sink (traffic terminates here)
			tdg.AddSink(current)
			continue
		}

		if len(entry.NextHops) == 0 {
			// Discard/blackhole
			tdg.AddSink(current)
			continue
		}

		for _, nh := range entry.NextHops {
			weight := nh.Weight
			if len(entry.NextHops) == 1 {
				weight = 1.0
			}

			// Ensure target node exists
			tdg.AddNode(nh.Node, "default", "fib_lookup", PrefixClassIDFromPacketClass(packetClass))

			// Add edge: current -> nh.Node with weight
			if _, err := tdg.AddEdge(current, nh.Node, weight); err != nil {
				panic(fmt.Sprintf("BuildTDG: AddEdge(%s, %s): %v", current, nh.Node, err))
			}

			if !visited[nh.Node] {
				queue = append(queue, nh.Node)
			}
		}
	}

	return tdg
}

// representativeAddr extracts a representative address from a PrefixSet.
func representativeAddr(set model.PrefixSet) netip.Addr {
	switch s := set.(type) {
	case model.ExactPrefixSet:
		return s.Prefix.Addr()
	case model.AnyPrefixSet:
		return netip.MustParseAddr("0.0.0.0")
	}
	// For other types, try string representation
	str := set.String()
	if str == "" || str == "any" {
		return netip.MustParseAddr("0.0.0.0")
	}
	if pfx, err := model.ParsePrefix(str); err == nil {
		return pfx.Addr()
	}
	return netip.Addr{}
}

// PrefixClassIDFromPacketClass extracts a PrefixClassID from a PacketClass.
func PrefixClassIDFromPacketClass(pc model.PacketClass) model.PrefixClassID {
	return pc.PrefixClassID
}
