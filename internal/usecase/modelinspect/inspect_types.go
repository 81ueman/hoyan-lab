package modelinspect

import (
	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

type Request struct {
	TopologyPath     string
	Node             string
	Prefix           string
	From             string
	To               string
	Protocol         string
	DstPort          int
	StrictConfig     bool
	MaxPrefixClasses int
}

type PrefixClassRow struct {
	ClassID           model.PrefixClassID `json:"class_id"`
	Space             string              `json:"space"`
	MatchedPredicates []string            `json:"matched_predicates,omitempty"`
}

type PacketClassRow struct {
	ClassID           model.PacketClassID `json:"class_id"`
	PrefixClassID     model.PrefixClassID `json:"prefix_class_id"`
	Space             string              `json:"space"`
	Protocol          string              `json:"protocol,omitempty"`
	SrcPort           string              `json:"src_port,omitempty"`
	DstPort           string              `json:"dst_port,omitempty"`
	IngressInterface  string              `json:"ingress_interface,omitempty"`
	EgressInterface   string              `json:"egress_interface,omitempty"`
	MatchedPredicates []string            `json:"matched_predicates,omitempty"`
}

type RIBRow struct {
	Node                  string   `json:"node"`
	Prefix                string   `json:"prefix"`
	SourceKind            string   `json:"source_kind,omitempty"`
	ConnectedClass        string   `json:"connected_class,omitempty"`
	OSPFRouteType         string   `json:"ospf_route_type,omitempty"`
	Metric                *int     `json:"metric,omitempty"`
	RouteInterface        string   `json:"interface,omitempty"`
	NextHopNode           string   `json:"next_hop_node,omitempty"`
	NextHopAddr           string   `json:"next_hop_addr,omitempty"`
	OriginNode            string   `json:"origin_node,omitempty"`
	FromNode              string   `json:"from_node,omitempty"`
	PathNodes             []string `json:"path_nodes,omitempty"`
	PathLinks             []string `json:"path_links,omitempty"`
	ASPath                []uint32 `json:"as_path,omitempty"`
	Communities           []string `json:"communities,omitempty"`
	OriginCode            *string  `json:"origin_code,omitempty"`
	LocalPref             *int     `json:"local_pref,omitempty"`
	MED                   *int     `json:"med,omitempty"`
	LearnedIBGP           *bool    `json:"learned_ibgp,omitempty"`
	Invalid               *bool    `json:"invalid,omitempty"`
	AggregateContributors []string `json:"aggregate_contributors,omitempty"`
	Condition             string   `json:"condition,omitempty"`
	SelectedCondition     string   `json:"selected_condition,omitempty"`
	BaseCondition         string   `json:"base_condition,omitempty"`
}

type FIBRow struct {
	Node             string   `json:"node"`
	Prefix           string   `json:"prefix"`
	SourceKind       string   `json:"source_kind,omitempty"`
	Discard          bool     `json:"discard,omitempty"`
	ConnectedClass   string   `json:"connected_class,omitempty"`
	Interface        string   `json:"interface,omitempty"`
	NextHop          string   `json:"next_hop_node,omitempty"`
	RawNextHop       string   `json:"raw_next_hop,omitempty"`
	NextHopAddress   string   `json:"next_hop_addr,omitempty"`
	ResolutionStatus string   `json:"resolution_status,omitempty"`
	ResolutionReason string   `json:"resolution_reason,omitempty"`
	Rank             int      `json:"rank"`
	GroupID          string   `json:"group_id,omitempty"`
	Equivalent       bool     `json:"equivalent"`
	PathNodes        []string `json:"path_nodes,omitempty"`
	PathLinks        []string `json:"path_links,omitempty"`
	Cost             int      `json:"cost"`
	Condition        string   `json:"condition,omitempty"`
}

type SymbolicPacketInspect struct {
	From               string                               `json:"from"`
	To                 string                               `json:"to"`
	Protocol           string                               `json:"protocol"`
	DstPort            int                                  `json:"dst_port,omitempty"`
	Reachable          string                               `json:"reachable_condition"`
	Unreachable        string                               `json:"unreachable_condition"`
	Reason             string                               `json:"reason,omitempty"`
	Paths              []SymbolicPacketInspectPath          `json:"paths,omitempty"`
	Blocked            []SymbolicPacketBlockedPath          `json:"blocked_paths,omitempty"`
	UnreachableReasons []SymbolicPacketInspectBlockedReason `json:"unreachable_reasons,omitempty"`
}

type SymbolicPacketInspectPath struct {
	PathNodes []string                     `json:"path_nodes,omitempty"`
	PathLinks []string                     `json:"path_links,omitempty"`
	Cost      int                          `json:"cost"`
	Condition string                       `json:"condition,omitempty"`
	States    []SymbolicPacketInspectState `json:"states,omitempty"`
}

type SymbolicPacketInspectState struct {
	Node             string   `json:"node"`
	IngressInterface string   `json:"ingress_interface,omitempty"`
	Condition        string   `json:"condition,omitempty"`
	PathNodes        []string `json:"path_nodes,omitempty"`
	PathLinks        []string `json:"path_links,omitempty"`
	Cost             int      `json:"cost"`
}

type SymbolicPacketBlockedPath struct {
	PathNodes     []string               `json:"path_nodes,omitempty"`
	PathLinks     []string               `json:"path_links,omitempty"`
	Cost          int                    `json:"cost"`
	Condition     string                 `json:"condition,omitempty"`
	Reason        string                 `json:"reason,omitempty"`
	ACL           string                 `json:"acl,omitempty"`
	RuleSeq       int                    `json:"rule_seq,omitempty"`
	Action        model.ACLAction        `json:"action,omitempty"`
	DefaultAction model.ACLDefaultAction `json:"default_action,omitempty"`
	Node          string                 `json:"node,omitempty"`
	Interface     string                 `json:"interface,omitempty"`
	Stage         string                 `json:"stage,omitempty"`
	Source        model.ConfigSource     `json:"source,omitempty"`
}

type SymbolicPacketInspectBlockedReason struct {
	Kind          string                 `json:"kind"`
	Node          string                 `json:"node,omitempty"`
	Link          string                 `json:"link,omitempty"`
	Interface     string                 `json:"interface,omitempty"`
	PolicyName    string                 `json:"policy_name,omitempty"`
	ACLName       string                 `json:"acl_name,omitempty"`
	RuleSeq       int                    `json:"rule_seq,omitempty"`
	Action        model.ACLAction        `json:"action,omitempty"`
	DefaultAction model.ACLDefaultAction `json:"default_action,omitempty"`
	PolicyRaw     string                 `json:"policy_raw,omitempty"`
	PathNodes     []string               `json:"path_nodes,omitempty"`
	PathLinks     []string               `json:"path_links,omitempty"`
	Cost          int                    `json:"cost"`
	Condition     string                 `json:"condition,omitempty"`
	Message       string                 `json:"message,omitempty"`
}

type SymbolicRouteInspect struct {
	From              string                     `json:"from"`
	Prefix            string                     `json:"prefix"`
	ClassID           model.PrefixClassID        `json:"class_id"`
	Space             string                     `json:"space"`
	MatchedPredicates []string                   `json:"matched_predicates,omitempty"`
	Reachable         string                     `json:"reachable_condition"`
	Unreachable       string                     `json:"unreachable_condition"`
	Reason            string                     `json:"reason,omitempty"`
	Paths             []SymbolicRouteInspectPath `json:"paths,omitempty"`
}

type SymbolicRouteInspectPath struct {
	PathNodes []string `json:"path_nodes,omitempty"`
	PathLinks []string `json:"path_links,omitempty"`
	Cost      int      `json:"cost"`
	Condition string   `json:"condition,omitempty"`
}

type RIBResult struct {
	Protocol model.RouteSourceKind
	Rows     []RIBRow
}

type FIBResult struct {
	Rows []FIBRow
}

type PrefixClassesResult struct {
	Stats   model.PrefixUniverseStats
	Classes []PrefixClassRow
}

type PacketClassesResult struct {
	Classes []PacketClassRow
}

type SymbolicRouteResult struct {
	Results []SymbolicRouteInspect
}

func ptr[T any](v T) *T {
	return &v
}

func condString(cond failure.Cond) string {
	if cond == nil {
		return ""
	}
	return cond.String()
}

func prefixSetString(set model.PrefixSet) string {
	if set == nil {
		return ""
	}
	return set.String()
}

func portSetInspectString(set model.PortSet) string {
	if set == nil {
		return "any"
	}
	return set.String()
}
