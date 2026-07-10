package traffic

import (
	"net/netip"
)

// DefaultFlowBytes is the assumed packet/flow size in bytes used when
// per-flow byte counters are not available.
const DefaultFlowBytes = 1500

// TrafficNextHop represents a next-hop in the traffic simulation FIB.
type TrafficNextHop struct {
	Node   string  `json:"node" yaml:"node"`
	Weight float64 `json:"weight" yaml:"weight"` // 0.0-1.0 for ECMP distribution
}

// TrafficFIBEntry is a FIB entry used for traffic simulation.
type TrafficFIBEntry struct {
	Prefix   netip.Prefix     `json:"prefix" yaml:"prefix"`
	NextHops []TrafficNextHop `json:"next_hops" yaml:"next_hops"`
}

// FIBTable maps node name to its traffic FIB entries.
type FIBTable map[string][]TrafficFIBEntry
