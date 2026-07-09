package observation

import "github.com/81ueman/hoyan-lab/internal/domain/model"

// RouteMetadata holds shared metadata fields present in both RIB and FIB entries.
// These fields are duplicated across RIB and FIB snapshots to avoid forcing
// callers to unwrap an intermediate struct, while the shared type ensures
// they stay in sync when new fields are added.
type RouteMetadata struct {
	// SAFI is the Subsequent Address Family Identifier (e.g. "unicast",
	// "multicast", "vpn-unicast"). Zero value means unknown / not reported.
	SAFI string `json:"safi,omitempty"`
	// TableID is the routing table identifier as reported by the device.
	TableID string `json:"table_id,omitempty"`
	// TableName is a human-friendly name for the routing table.
	TableName string `json:"table_name,omitempty"`
	// ProtocolInstance identifies the specific protocol process or
	// instance (e.g. "BGP 65000", "OSPF 100", "OSPFv3 1").
	ProtocolInstance string `json:"protocol_instance,omitempty"`
	// Age is the route age as a human-readable string (e.g. "00:12:34",
	// "1h2m3s").
	Age string `json:"age,omitempty"`
	// AgeSeconds is the route age in seconds.
	AgeSeconds int `json:"age_seconds,omitempty"`
	// Tag is an administratively assigned route tag value.
	Tag uint32 `json:"tag,omitempty"`
	// InstalledReason describes why the route was or was not installed
	// into the FIB (e.g. "active", "inactive", "fib", "not_selected").
	InstalledReason string `json:"installed_reason,omitempty"`
	// Raw holds vendor-specific attributes that do not have a dedicated
	// field in this schema. It is never compared by default.
	Raw map[string]any `json:"raw,omitempty"`
}

type RouteSource struct {
	Protocol model.RouteSourceKind `json:"protocol"`
	Origin   string                `json:"origin,omitempty"`
}

// NextHop is an observed RIB/FIB next-hop as reported by a device or simulator.
// It may include observation-specific metadata such as interface, weight, and
// resolution state; the control-plane forwarding reference is route.NextHop.
type NextHop struct {
	Address   string       `json:"address,omitempty"`
	Interface string       `json:"interface,omitempty"`
	Node      model.NodeID `json:"node,omitempty"`
	Weight    int          `json:"weight,omitempty"`

	Resolution *NextHopResolution `json:"resolution,omitempty"`

	// Resolved contains the resolved next-hop chain for recursively
	// resolved routes (e.g. a BGP next-hop that resolves through an IGP).
	Resolved []NextHop `json:"resolved,omitempty"`
	// Raw holds vendor-specific attributes that do not have a dedicated
	// field in this schema. It is never compared by default.
	Raw map[string]any `json:"raw,omitempty"`
}

type NextHopResolution struct {
	Status string `json:"status,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type ModelRouteInfo struct {
	Conditions ModelConditions `json:"conditions,omitempty"`
	Provenance RouteProvenance `json:"provenance,omitempty"`
	Decision   *DecisionInfo   `json:"decision,omitempty"`
	Resolution *ResolutionInfo `json:"resolution,omitempty"`
}

type ModelConditions struct {
	Base      string `json:"base,omitempty"`
	Eligible  string `json:"eligible,omitempty"`
	Selected  string `json:"selected,omitempty"`
	Installed string `json:"installed,omitempty"`
}

type RouteProvenance struct {
	OriginNode model.NodeID   `json:"origin_node,omitempty"`
	FromNode   model.NodeID   `json:"from_node,omitempty"`
	PathNodes  []model.NodeID `json:"path_nodes,omitempty"`
	PathLinks  []string       `json:"path_links,omitempty"`
	Source     string         `json:"source,omitempty"`
	Inputs     []string       `json:"inputs,omitempty"`
}

type DecisionInfo struct {
	Rank       int    `json:"rank,omitempty"`
	GroupID    string `json:"group_id,omitempty"`
	Equivalent bool   `json:"equivalent,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type ResolutionInfo struct {
	Status string `json:"status,omitempty"`
	Reason string `json:"reason,omitempty"`
}
