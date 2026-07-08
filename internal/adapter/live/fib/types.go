package fib

import (
	"context"
	"sort"

	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

type FIBEntry = observation.FIBEntry
type FIB = observation.FIB
type NextHop = observation.NextHop
type Options = observation.Options
type UnsupportedNodesError = observation.UnsupportedNodesError

type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

var SortRoutes = observation.SortFIBEntriesForCompare
var CanonicalProtocol = observation.CanonicalProtocol

func sortedFIBs(fibs []FIB) []FIB {
	byKey := map[string]FIB{}
	for _, fib := range fibs {
		existing := byKey[fib.Key()]
		if existing.Node == "" {
			existing.Node = fib.Node
			existing.VRF = fib.VRF
		}
		existing.Entries = append(existing.Entries, fib.Entries...)
		byKey[fib.Key()] = existing
	}
	out := make([]FIB, 0, len(byKey))
	for _, fib := range byKey {
		observation.SortFIBEntriesForCompare(fib.Entries)
		out = append(out, fib)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Key() < out[j].Key()
	})
	return out
}
