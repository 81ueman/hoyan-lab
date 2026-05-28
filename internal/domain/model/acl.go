package model

type ACLAction string

const (
	ACLPermit ACLAction = "permit"
	ACLDeny   ACLAction = "deny"
)

type ACLDefaultAction string

const (
	ACLDefaultPermit ACLDefaultAction = "permit"
	ACLDefaultDeny   ACLDefaultAction = "deny"
)

type ACLRule struct {
	Seq    int          `yaml:"seq" json:"seq"`
	Action ACLAction    `yaml:"action" json:"action"`
	Match  PacketSpec   `yaml:"-" json:"-"`
	Source ConfigSource `yaml:"source,omitempty" json:"source,omitempty"`
}

type ACL struct {
	Name          string           `yaml:"name" json:"name"`
	Node          string           `yaml:"node" json:"node"`
	Vendor        DeviceKind       `yaml:"vendor" json:"vendor"`
	Rules         []ACLRule        `yaml:"rules" json:"rules"`
	DefaultAction ACLDefaultAction `yaml:"default_action" json:"default_action"`
	Source        ConfigSource     `yaml:"source,omitempty" json:"source,omitempty"`
}

type ACLBinding struct {
	Node      string       `yaml:"node" json:"node"`
	Interface string       `yaml:"interface" json:"interface"`
	Direction string       `yaml:"direction" json:"direction"`
	ACLName   string       `yaml:"acl_name" json:"acl_name"`
	Source    ConfigSource `yaml:"source,omitempty" json:"source,omitempty"`
}
