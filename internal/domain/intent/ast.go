package intent

type Document struct {
	Version   string              `yaml:"version" json:"version"`
	Vars      map[string]any      `yaml:"vars,omitempty" json:"vars,omitempty"`
	Snapshots map[string]Snapshot `yaml:"snapshots,omitempty" json:"snapshots,omitempty"`
	Scenarios map[string]Scenario `yaml:"scenarios,omitempty" json:"scenarios,omitempty"`
	Intents   []Intent            `yaml:"intents,omitempty" json:"intents,omitempty"`
}

type Snapshot struct {
	Lab string `yaml:"lab" json:"lab"`
}

type Scenario struct {
	Snapshot string             `yaml:"snapshot" json:"snapshot"`
	Failures FailureConstraints `yaml:"failures,omitempty" json:"failures,omitempty"`
}

type FailureConstraints struct {
	Max              int      `yaml:"max,omitempty" json:"max,omitempty"`
	IncludeLinkRoles []string `yaml:"include_link_roles,omitempty" json:"include_link_roles,omitempty"`
	ExcludeLinkRoles []string `yaml:"exclude_link_roles,omitempty" json:"exclude_link_roles,omitempty"`
	IncludeLinks     []string `yaml:"include_links,omitempty" json:"include_links,omitempty"`
	ExcludeLinks     []string `yaml:"exclude_links,omitempty" json:"exclude_links,omitempty"`
	IncludeNodeRoles []string `yaml:"include_node_roles,omitempty" json:"include_node_roles,omitempty"`
	ExcludeNodeRoles []string `yaml:"exclude_node_roles,omitempty" json:"exclude_node_roles,omitempty"`
	IncludeNodes     []string `yaml:"include_nodes,omitempty" json:"include_nodes,omitempty"`
	ExcludeNodes     []string `yaml:"exclude_nodes,omitempty" json:"exclude_nodes,omitempty"`
}

type Intent struct {
	Name     string         `yaml:"name" json:"name"`
	Scenario string         `yaml:"scenario,omitempty" json:"scenario,omitempty"`
	Forall   map[string]any `yaml:"forall,omitempty" json:"forall,omitempty"`
	RCL      *RCLExpr       `yaml:"rcl" json:"rcl"`
	Group    map[string]any `yaml:"-" json:"group,omitempty"`
}

// RCLExpr is a recursive expression node for RCL intents.
// Exactly one field should be set (tagged union).
type RCLExpr struct {
	Guard           *GuardExpr           `yaml:"guard,omitempty" json:"guard,omitempty"`
	Forall          *ForallExpr          `yaml:"forall,omitempty" json:"forall,omitempty"`
	And             []RCLExpr            `yaml:"and,omitempty" json:"and,omitempty"`
	Or              []RCLExpr            `yaml:"or,omitempty" json:"or,omitempty"`
	Not             *RCLExpr             `yaml:"not,omitempty" json:"not,omitempty"`
	Imply           [2]*RCLExpr          `yaml:"imply,omitempty" json:"imply,omitempty"`
	RIBEq           *RIBEqExpr           `yaml:"rib_eq,omitempty" json:"rib_eq,omitempty"`
	RIBEval         *RIBEvalExpr         `yaml:"rib_eval,omitempty" json:"rib_eval,omitempty"`
	PacketReachable *PacketReachableExpr `yaml:"packet_reachable,omitempty" json:"packet_reachable,omitempty"`
}

type GuardExpr struct {
	Where  map[string]any `yaml:"where" json:"where"`   // route predicate（既存のwhere形式）
	Intent RCLExpr        `yaml:"intent" json:"intent"` // 入れ子のintent
}

type ForallExpr struct {
	Var    string   `yaml:"var" json:"var"`
	In     []string `yaml:"in,omitempty" json:"in,omitempty"` // 省略時は全値
	Intent RCLExpr  `yaml:"intent" json:"intent"`
}

type RIBEqExpr struct {
	Left  string         `yaml:"left" json:"left"`   // "PRE" or "POST" → スナップショット名に
	Right string         `yaml:"right" json:"right"` // "PRE" or "POST"
	Where map[string]any `yaml:"where,omitempty" json:"where,omitempty"`
}

type RIBEvalExpr struct {
	Snapshot  string         `yaml:"snapshot,omitempty" json:"snapshot,omitempty"` // デフォルトscenario.snapshot
	Where     map[string]any `yaml:"where,omitempty" json:"where,omitempty"`
	Aggregate string         `yaml:"aggregate" json:"aggregate"` // e.g. "count()", "distCnt(nexthop)", "distVals(localPref)"
	Eq        []any          `yaml:"eq,omitempty" json:"eq,omitempty"`
	Ne        []any          `yaml:"ne,omitempty" json:"ne,omitempty"`
	Gt        *int           `yaml:"gt,omitempty" json:"gt,omitempty"`
	Gte       *int           `yaml:"gte,omitempty" json:"gte,omitempty"`
	Lt        *int           `yaml:"lt,omitempty" json:"lt,omitempty"`
	Lte       *int           `yaml:"lte,omitempty" json:"lte,omitempty"`
}

type PacketReachableExpr struct {
	From     string `yaml:"from" json:"from"`
	VRF      string `yaml:"vrf,omitempty" json:"vrf,omitempty"`
	To       string `yaml:"to" json:"to"`
	Protocol string `yaml:"protocol" json:"protocol"`
	DstPort  int    `yaml:"dst_port,omitempty" json:"dst_port,omitempty"`
	Expect   bool   `yaml:"expect" json:"expect"`
}

type ExpandedDocument struct {
	Version   string              `json:"version"`
	Snapshots map[string]Snapshot `json:"snapshots,omitempty"`
	Scenarios map[string]Scenario `json:"scenarios,omitempty"`
	Intents   []Intent            `json:"intents"`
}
