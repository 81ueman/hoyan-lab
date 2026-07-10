package traffic

import (
	"math"
	"runtime"
	"sort"
	"sync"

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
		totalBytes := uint64(len(flows)) * DefaultFlowBytes
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

// SimulateMultiSnapshot simulates multiple traffic snapshots and computes diffs.
// Each snapshot is a pair of label and FIB table representing a network state.
//
// If flows are provided and hash mode is configured, per-flow simulation is used.
// Otherwise, uniform-weight bulk simulation is used.
func (ts *TrafficSimulator) SimulateMultiSnapshot(
	rootNode string,
	packetClass model.PacketClass,
	snapshots []SnapshotDef,
	flows []Flow,
) model.MultiSnapshotResult {
	result := model.MultiSnapshotResult{}

	for _, snap := range snapshots {
		var linkLoads map[string]uint64
		if ts.config.ECMPMode == ECMPModeHash && len(flows) > 0 {
			linkLoads = ts.SimulateClassWithFlows(rootNode, packetClass, snap.FIBs, flows)
		} else {
			linkLoads = ts.SimulateClass(rootNode, packetClass, snap.FIBs, snap.TotalBytes)
		}
		result.Snapshots = append(result.Snapshots, model.TrafficResult{
			Label:     snap.Label,
			LinkLoads: linkLoads,
		})
	}

	// Compute diffs between consecutive snapshots
	result.Diffs = ComputeDiffs(result.Snapshots)

	return result
}

// ComputeDiffs computes link load differences between consecutive snapshots.
// Each adjacent pair (snapshots[i-1], snapshots[i]) is compared and the resulting
// diffs are all returned in a single sorted slice.
func ComputeDiffs(snapshots []model.TrafficResult) []model.LinkLoadDiff {
	if len(snapshots) < 2 {
		return nil
	}

	var diffs []model.LinkLoadDiff

	for i := 1; i < len(snapshots); i++ {
		prev := snapshots[i-1]
		curr := snapshots[i]

		// Collect all link names across this pair of snapshots
		allLinks := map[string]bool{}
		for link := range prev.LinkLoads {
			allLinks[link] = true
		}
		for link := range curr.LinkLoads {
			allLinks[link] = true
		}

		for link := range allLinks {
			before := int64(prev.LinkLoads[link])
			after := int64(curr.LinkLoads[link])

			if before == after {
				continue
			}

			var changePct float64
			if before > 0 {
				changePct = float64(after-before) / float64(before) * 100.0
				changePct = math.Round(changePct*100) / 100 // Round to 2 decimal places
			} else {
				// before == 0, new traffic appeared
				changePct = math.Inf(1)
			}

			diffs = append(diffs, model.LinkLoadDiff{
				LinkName:  link,
				Before:    uint64(before),
				After:     uint64(after),
				ChangePct: changePct,
			})
		}
	}

	sort.Slice(diffs, func(i, j int) bool {
		if diffs[i].LinkName == diffs[j].LinkName {
			return diffs[i].Before < diffs[j].Before
		}
		return diffs[i].LinkName < diffs[j].LinkName
	})

	return diffs
}

// SimulateParallel simulates traffic for multiple equivalence classes in parallel.
// Each EC is independently simulated on its own goroutine, bounded by workers.
func (ts *TrafficSimulator) SimulateParallel(
	rootNode string,
	ecs []FlowEquivalenceClass,
	fibs FIBTable,
	workers int,
) map[string]uint64 {
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}

	var mu sync.Mutex
	linkBytes := map[string]uint64{}
	ch := make(chan map[string]uint64, workers)
	sem := make(chan struct{}, workers)

	go func() {
		var wg sync.WaitGroup
		for _, ec := range ecs {
			sem <- struct{}{}
			wg.Add(1)
			go func(ec FlowEquivalenceClass) {
				defer func() { <-sem }()
				defer wg.Done()

				pc := model.PacketClass{
					PrefixClassID: ec.Key.PrefixClassID,
					DstSet:        ec.DstSet,
				}
				// If we have flows in the EC, simulate with flows
				if len(ec.Flows) > 0 && ts.config.ECMPMode == ECMPModeHash {
					flows := make([]Flow, len(ec.Flows))
					for i, sf := range ec.Flows {
						flows[i] = sf.Flow
					}
					result := ts.SimulateClassWithFlows(rootNode, pc, fibs, flows)
					ch <- result
				} else {
					// Use total bytes if available, otherwise fallback
					bytes := ec.TotalBytes
					if bytes == 0 {
						bytes = uint64(len(ec.Flows)) * DefaultFlowBytes
					}
					result := ts.SimulateClass(rootNode, pc, fibs, bytes)
					ch <- result
				}
			}(ec)
		}
		wg.Wait()
		close(ch)
	}()

	for result := range ch {
		for link, bytes := range result {
			mu.Lock()
			linkBytes[link] += bytes
			mu.Unlock()
		}
	}

	return linkBytes
}

// SnapshotDef defines a single snapshot for multi-snapshot simulation.
type SnapshotDef struct {
	Label      string   `json:"label" yaml:"label"`
	FIBs       FIBTable `json:"fibs" yaml:"fibs"`
	TotalBytes uint64   `json:"total_bytes" yaml:"total_bytes"`
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
	queue := []nodeLoad{{node: rootNode, bytes: DefaultFlowBytes}}

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
