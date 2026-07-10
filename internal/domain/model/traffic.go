package model

import "net/netip"

// Flow represents a network flow (5-tuple).
type Flow struct {
	SrcIP    netip.Addr
	DstIP    netip.Addr
	Protocol string
	SrcPort  int
	DstPort  int
}

// LocatedFlow represents a flow with its ingress location.
type LocatedFlow struct {
	Flow
	IngressNode string
	IngressIntf string
	Bytes       uint64
}

// LinkLoad represents the load on a single link.
type LinkLoad struct {
	LinkName string
	Bytes    uint64
}

// TrafficResult holds the traffic simulation result for one snapshot.
type TrafficResult struct {
	// Label identifies this snapshot (e.g., "baseline", "after-change").
	Label string `json:"label,omitempty" yaml:"label,omitempty"`
	// LinkLoads maps link name -> link load.
	LinkLoads map[string]LinkLoad `json:"link_loads" yaml:"link_loads"`
}

// LinkLoadDiff represents the change in load on a single link between
// two consecutive snapshots.
type LinkLoadDiff struct {
	LinkName  string  `json:"link_name" yaml:"link_name"`
	Before    uint64  `json:"before" yaml:"before"`
	After     uint64  `json:"after" yaml:"after"`
	ChangePct float64 `json:"change_pct" yaml:"change_pct"`
}

// MultiSnapshotResult holds results from simulating multiple traffic snapshots
// against the same topology.
type MultiSnapshotResult struct {
	Snapshots []TrafficResult `json:"snapshots" yaml:"snapshots"`
	Diffs     []LinkLoadDiff  `json:"diffs" yaml:"diffs"`
}
