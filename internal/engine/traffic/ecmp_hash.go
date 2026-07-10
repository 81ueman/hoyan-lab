package traffic

import (
	"encoding/binary"
	"hash/fnv"
	"net/netip"
)

// ECMPMode controls how ECMP next-hops are selected.
type ECMPMode int

const (
	// ECMPModeUniform uses uniform weights for ECMP distribution (Phase 1 default).
	ECMPModeUniform ECMPMode = iota
	// ECMPModeHash uses 5-tuple hashing for flow-level ECMP distribution.
	ECMPModeHash
)

// Flow represents a network flow (5-tuple).
type Flow struct {
	SrcIP    netip.Addr
	DstIP    netip.Addr
	Protocol string
	SrcPort  uint16
	DstPort  uint16
}

// SelectECMPMember selects an ECMP next-hop index using 5-tuple hashing.
// Returns the index into the nextHops slice.
func SelectECMPMember(flow Flow, nextHops []TrafficNextHop) int {
	if len(nextHops) == 1 {
		return 0
	}
	h := fnv.New64a()

	// Hash the 5-tuple: srcIP, dstIP, protocol, srcPort, dstPort
	srcBytes := flow.SrcIP.AsSlice()
	if len(srcBytes) == 4 {
		// Pad IPv4 to 16 bytes for consistent hashing
		padded := make([]byte, 16)
		copy(padded[12:], srcBytes)
		srcBytes = padded
	} else if len(srcBytes) == 16 {
		// Already 16 bytes (IPv6)
	}
	h.Write(srcBytes)

	dstBytes := flow.DstIP.AsSlice()
	if len(dstBytes) == 4 {
		padded := make([]byte, 16)
		copy(padded[12:], dstBytes)
		dstBytes = padded
	} else if len(dstBytes) == 16 {
		// Already 16 bytes (IPv6)
	}
	h.Write(dstBytes)

	// Write protocol as a 4-byte field
	protoBytes := make([]byte, 4)
	copy(protoBytes, []byte(flow.Protocol))
	h.Write(protoBytes)

	// Write ports
	portBytes := make([]byte, 4)
	binary.BigEndian.PutUint16(portBytes[0:2], flow.SrcPort)
	binary.BigEndian.PutUint16(portBytes[2:4], flow.DstPort)
	h.Write(portBytes)

	idx := h.Sum64() % uint64(len(nextHops))
	return int(idx)
}
