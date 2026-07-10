package traffic

import (
	"math"
	"sort"

	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

// WhatIfResult holds the result of a what-if failure simulation.
type WhatIfResult struct {
	Failure   failure.Set
	LinkLoads map[string]model.LinkLoad
	Diffs     []LinkLoadChange
}

// LinkLoadChange represents a change in link load between base and failure.
type LinkLoadChange struct {
	LinkName  string
	Before    uint64
	After     uint64
	Delta     int64
	DeltaPct  float64
}

// WhatIfSimulator simulates traffic under failure conditions and compares
// with the base (no-failure) case.
type WhatIfSimulator struct {
	config SimulatorConfig
	base   map[string]model.LinkLoad // base link loads for comparison
}

// NewWhatIfSimulator creates a new WhatIfSimulator.
func NewWhatIfSimulator(config SimulatorConfig) *WhatIfSimulator {
	return &WhatIfSimulator{
		config: config,
		base:   nil,
	}
}

// Simulate simulates traffic for a given failure set and compares it with
// the base case. If no base case has been recorded, the current simulation
// is stored as the base and returned with no diffs.
//
// rootNode is the ingress node for traffic simulation.
// ecs is the list of flow equivalence classes to simulate.
// cache is used to build and cache TDGs.
// fibs is the FIB table to use for simulations (same for base and failure).
func (ws *WhatIfSimulator) Simulate(
	rootNode string,
	failSet failure.Set,
	ecs []FlowEquivalenceClass,
	cache *TDGCache,
	fibs FIBTable,
) *WhatIfResult {
	if rootNode == "" {
		return nil
	}

	// Build or retrieve base TDGs and compute base link loads
	// We need to simulate per-class and then aggregate
	failedTDGByClass := make(map[int]*model.TDG)
	baseLoads := make(map[string]uint64)

	for _, ec := range ecs {
		pc := model.PacketClass{
			PrefixClassID: ec.Key.PrefixClassID,
			DstSet:        ec.DstSet,
		}

		// Get or build the base TDG from cache
		baseTDG := cache.GetOrBuild(rootNode, pc, fibs)

		// If no failure, just traverse the base TDG
		if len(failSet.Links) == 0 && len(failSet.Nodes) == 0 {
			loads := Traverse(baseTDG, ec.TotalBytes)
			for link, bytes := range loads {
				baseLoads[link] += bytes
			}
			continue
		}

		// Apply failure to get the failed TDG
		failedTDG := cache.ApplyFailure(baseTDG, failSet)
		failedTDGByClass[int(pc.PrefixClassID)] = failedTDG

		// Compute base loads from the original (unfailed) TDG
		baseLoadsForClass := Traverse(baseTDG, ec.TotalBytes)
		for link, bytes := range baseLoadsForClass {
			baseLoads[link] += bytes
		}
	}

	// If no failure, this is the base case
	if len(failSet.Links) == 0 && len(failSet.Nodes) == 0 {
		result := &WhatIfResult{
			Failure:   failSet,
			LinkLoads: toLinkLoadsMap(baseLoads),
			Diffs:     nil,
		}
		ws.base = result.LinkLoads
		return result
	}

	// Compute failed link loads
	failedLoads := make(map[string]uint64)
	for _, ec := range ecs {
		pc := model.PacketClass{
			PrefixClassID: ec.Key.PrefixClassID,
			DstSet:        ec.DstSet,
		}
		failedTDG, ok := failedTDGByClass[int(pc.PrefixClassID)]
		if !ok {
			continue
		}
		loads := Traverse(failedTDG, ec.TotalBytes)
		for link, bytes := range loads {
			failedLoads[link] += bytes
		}
	}

	// Generate diffs comparing base vs failed
	diffs := computeLinkLoadChanges(baseLoads, failedLoads)

	return &WhatIfResult{
		Failure:   failSet,
		LinkLoads: toLinkLoadsMap(failedLoads),
		Diffs:     diffs,
	}
}

// toLinkLoadsMap converts a map[string]uint64 to map[string]model.LinkLoad.
func toLinkLoadsMap(raw map[string]uint64) map[string]model.LinkLoad {
	out := make(map[string]model.LinkLoad, len(raw))
	for link, bytes := range raw {
		out[link] = model.LinkLoad{LinkName: link, Bytes: bytes}
	}
	return out
}

// computeLinkLoadChanges computes diffs between base and failed link loads.
func computeLinkLoadChanges(base, failed map[string]uint64) []LinkLoadChange {
	allLinks := make(map[string]bool)
	for link := range base {
		allLinks[link] = true
	}
	for link := range failed {
		allLinks[link] = true
	}

	var changes []LinkLoadChange
	for link := range allLinks {
		before := base[link]
		after := failed[link]
		delta := int64(after) - int64(before)
		if delta == 0 {
			continue
		}

		var deltaPct float64
		if before > 0 {
			deltaPct = float64(delta) / float64(before) * 100.0
			deltaPct = math.Round(deltaPct*100) / 100
		} else {
			// New traffic appeared on this link
			deltaPct = math.Inf(1)
		}

		changes = append(changes, LinkLoadChange{
			LinkName: link,
			Before:   before,
			After:    after,
			Delta:    delta,
			DeltaPct: deltaPct,
		})
	}

	sort.Slice(changes, func(i, j int) bool {
		return changes[i].LinkName < changes[j].LinkName
	})

	return changes
}
