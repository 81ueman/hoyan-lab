package fib

import "github.com/81ueman/hoyan-lab/internal/domain/observation"

type FIBEntry = observation.FIBEntry
type NextHop = observation.NextHop
type Collector = observation.FIBCollector
type Runner = observation.FIBRunner
type Options = observation.Options
type UnsupportedNodesError = observation.UnsupportedNodesError

var SortRoutes = observation.SortFIBEntriesForCompare
var CanonicalProtocol = observation.CanonicalProtocol
