package traffic

import (
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

// linkBandwidth returns the bandwidth for a link, using the explicit bandwidth
// when set (non-zero), otherwise falling back to a role-based default.
func linkBandwidth(link model.Link, topo *model.Topology) uint64 {
	if link.Bandwidth > 0 {
		return link.Bandwidth
	}
	return defaultBandwidthForRole(link.Role)
}

// defaultBandwidthForRole returns a default bandwidth based on the link's role.
// Returns 0 if the role is unknown (meaning no bandwidth constraint).
func defaultBandwidthForRole(role string) uint64 {
	switch role {
	case "core":
		return 40_000_000_000 // 40 Gbps
	case "edge", "border":
		return 10_000_000_000 // 10 Gbps
	case "customer":
		return 1_000_000_000 // 1 Gbps
	default:
		return 0 // unknown
	}
}

// BandwidthOverride is a map of link name -> bandwidth in bps.
type BandwidthOverride map[string]uint64

// ApplyBandwidthOverrides applies bandwidth overrides to a topology's links.
func ApplyBandwidthOverrides(topo *model.Topology, overrides BandwidthOverride) {
	for i := range topo.Links {
		if bw, ok := overrides[topo.Links[i].Name]; ok {
			topo.Links[i].Bandwidth = bw
		}
	}
}
