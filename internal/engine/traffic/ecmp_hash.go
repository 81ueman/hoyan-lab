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

// padAddrTo16 converts an IP address to a 16-byte slice.
// IPv4 addresses are zero-padded in the first 12 bytes (IPv4-in-IPv6 mapping).
func padAddrTo16(addr netip.Addr) []byte {
	b := addr.AsSlice()
	if len(b) == 4 {
		padded := make([]byte, 16)
		copy(padded[12:], b)
		return padded
	}
	return b
}

// SelectECMPMember selects an ECMP next-hop index using 5-tuple hashing.
// Returns the index into the nextHops slice.
func SelectECMPMember(flow Flow, nextHops []TrafficNextHop) int {
	if len(nextHops) == 1 {
		return 0
	}
	h := fnv.New64a()

	// Hash the 5-tuple: srcIP, dstIP, protocol, srcPort, dstPort
	h.Write(padAddrTo16(flow.SrcIP))
	h.Write(padAddrTo16(flow.DstIP))

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
