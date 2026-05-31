package observation

import "github.com/81ueman/hoyan-lab/internal/domain/model"

type RouteSource struct {
	Protocol model.RouteSourceKind `json:"protocol"`
	Origin   string                `json:"origin,omitempty"`
}

type NextHop struct {
	Address   string       `json:"address,omitempty"`
	Interface string       `json:"interface,omitempty"`
	Node      model.NodeID `json:"node,omitempty"`
	Weight    int          `json:"weight,omitempty"`

	Resolution *NextHopResolution `json:"resolution,omitempty"`
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
