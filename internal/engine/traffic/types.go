package traffic

import (
	"net/netip"
)

// TrafficNextHop represents a next-hop in the traffic simulation FIB.
type TrafficNextHop struct {
	Node   string
	Weight float64 // 0.0-1.0 for ECMP distribution
}

// TrafficFIBEntry is a FIB entry used for traffic simulation.
type TrafficFIBEntry struct {
	Prefix   netip.Prefix
	NextHops []TrafficNextHop
}

// FIBTable maps node name to its traffic FIB entries.
type FIBTable map[string][]TrafficFIBEntry
