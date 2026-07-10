package model

import "net/netip"

// Flow is a 5-tuple representing a single traffic flow.
type Flow struct {
	SrcIP    netip.Addr
	DstIP    netip.Addr
	Protocol string // "tcp", "udp", "icmp"
	SrcPort  int
	DstPort  int
}

// LocatedFlow is a flow with network ingress point and traffic volume.
type LocatedFlow struct {
	Flow        Flow
	IngressNode string
	IngressIntf string
	Bytes       uint64
}

// FlowEquivalenceClass groups flows with identical forwarding behavior.
type FlowEquivalenceClass struct {
	ID          int
	PacketClass PacketClass // reuse existing
	IngressNode string
	IngressIntf string
	TotalBytes  uint64
	FlowCount   int
}

// LinkLoad represents traffic load on a single link.
type LinkLoad struct {
	LinkName string
	Bytes    uint64
	Capacity uint64 // bps
}

// TrafficResult is the output of a traffic simulation run.
type TrafficResult struct {
	LinkLoads map[string]LinkLoad
}
