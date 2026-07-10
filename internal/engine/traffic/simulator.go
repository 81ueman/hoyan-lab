package traffic

import (
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

// SimulatorConfig configures the traffic simulator behavior.
type SimulatorConfig struct {
	// ECMPMode controls how ECMP next-hops are selected.
	ECMPMode ECMPMode
}

// DefaultSimulatorConfig returns a default simulator configuration.
func DefaultSimulatorConfig() SimulatorConfig {
	return SimulatorConfig{
		ECMPMode: ECMPModeUniform,
	}
}

// TrafficSimulator simulates traffic distribution through the network.
type TrafficSimulator struct {
	config SimulatorConfig
}

// NewTrafficSimulator creates a new traffic simulator with the given config.
func NewTrafficSimulator(config SimulatorConfig) *TrafficSimulator {
	return &TrafficSimulator{config: config}
}

// SimulateClass simulates traffic for a single packet class through the network.
func (ts *TrafficSimulator) SimulateClass(
	rootNode string,
	packetClass model.PacketClass,
	fibs FIBTable,
	totalBytes uint64,
) map[string]uint64 {
	tdg := BuildTDG(rootNode, packetClass, fibs)
	return Traverse(tdg, totalBytes)
}

// SimulateClassWithFlows simulates traffic for a packet class using per-flow
// hash-based ECMP distribution. Each flow is individually hashed to an ECMP
// next-hop, providing flow-level consistency.
func (ts *TrafficSimulator) SimulateClassWithFlows(
	rootNode string,
	packetClass model.PacketClass,
	fibs FIBTable,
	flows []Flow,
) map[string]uint64 {
	if ts.config.ECMPMode != ECMPModeHash {
		// Fall back to uniform weight if hash mode not configured
		totalBytes := uint64(len(flows)) * 1500 // Assume ~1500 bytes per flow
		return ts.SimulateClass(rootNode, packetClass, fibs, totalBytes)
	}

	linkBytes := map[string]uint64{}

	// For hash mode, we need to process each flow individually through the TDG
	// but with hash-based ECMP selection at each hop.
	for _, flow := range flows {
		flowBytes := ts.simulateFlow(rootNode, packetClass, fibs, flow)
		for link, bytes := range flowBytes {
			linkBytes[link] += bytes
		}
	}
	return linkBytes
}

// simulateFlow simulates a single flow through the network using hash-based ECMP.
func (ts *TrafficSimulator) simulateFlow(
	rootNode string,
	packetClass model.PacketClass,
	fibs FIBTable,
	flow Flow,
) map[string]uint64 {
	linkBytes := map[string]uint64{}

	// Get destination address from the flow
	dstAddr := flow.DstIP
	if !dstAddr.IsValid() {
		// Fall back to representative address from packet class
		if packetClass.DstSet != nil {
			dstAddr = representativeAddr(packetClass.DstSet)
		}
	}
	if !dstAddr.IsValid() {
		return linkBytes
	}

	// Traverse the network hop by hop
	visited := map[string]bool{}
	queue := []nodeLoad{{node: rootNode, bytes: 1500}} // Assume 1500 bytes per flow

	for len(queue) > 0 {
		nl := queue[0]
		queue = queue[1:]
		if visited[nl.node] {
			continue
		}
		visited[nl.node] = true

		entry := lookupFIB(nl.node, dstAddr, fibs)
		if entry == nil || len(entry.NextHops) == 0 {
			continue
		}

		// Use hash-based ECMP to select one next-hop
		idx := SelectECMPMember(flow, entry.NextHops)
		if idx < 0 || idx >= len(entry.NextHops) {
			continue
		}
		nh := entry.NextHops[idx]

		link := linkName(nl.node, nh.Node)
		linkBytes[link] += nl.bytes

		if !visited[nh.Node] {
			queue = append(queue, nodeLoad{node: nh.Node, bytes: nl.bytes})
		}
	}

	return linkBytes
}

type nodeLoad struct {
	node  string
	bytes uint64
}
