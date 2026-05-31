package model

import "strings"

type BGPOriginCode string

const (
	BGPOriginIGP        BGPOriginCode = "igp"
	BGPOriginEGP        BGPOriginCode = "egp"
	BGPOriginIncomplete BGPOriginCode = "incomplete"
)

func NormalizeBGPOriginCode(origin BGPOriginCode) BGPOriginCode {
	switch strings.ToLower(strings.TrimSpace(string(origin))) {
	case "", "i", string(BGPOriginIGP):
		return BGPOriginIGP
	case "e", string(BGPOriginEGP):
		return BGPOriginEGP
	case "?", string(BGPOriginIncomplete):
		return BGPOriginIncomplete
	default:
		return BGPOriginCode(strings.ToLower(strings.TrimSpace(string(origin))))
	}
}

type BGPNeighbor struct {
	NetworkInstance NetworkInstanceID `yaml:"network_instance,omitempty" json:"network_instance,omitempty"`
	Address         string            `yaml:"address"`
	RemoteAS        uint32            `yaml:"remote_as"`
	Activated       bool              `yaml:"activated"`
	NextHopSelf     bool              `yaml:"next_hop_self"`
	PeerNode        string            `yaml:"peer_node"`
	ImportPolicy    string            `yaml:"import_policy"`
	ExportPolicy    string            `yaml:"export_policy"`
}

type PrefixList struct {
	Name  string           `yaml:"name"`
	Rules []PrefixListRule `yaml:"rules"`
}

type PrefixListRule struct {
	Seq    int       `yaml:"seq"`
	Action string    `yaml:"action"`
	Prefix string    `yaml:"prefix"`
	Ge     int       `yaml:"ge,omitempty"`
	Le     int       `yaml:"le,omitempty"`
	Match  PrefixSet `yaml:"-"`
}

type ASPathList struct {
	Name  string           `yaml:"name"`
	Rules []StringListRule `yaml:"rules"`
}

type CommunityList struct {
	Name  string           `yaml:"name"`
	Rules []StringListRule `yaml:"rules"`
}

type StringListRule struct {
	Seq     int    `yaml:"seq"`
	Action  string `yaml:"action"`
	Pattern string `yaml:"pattern"`
}

type RoutePolicy struct {
	Name  string            `yaml:"name"`
	Rules []RoutePolicyRule `yaml:"rules"`
}

type RoutePolicyRule struct {
	Seq                    int           `yaml:"seq"`
	Action                 string        `yaml:"action"`
	MatchPrefixList        string        `yaml:"match_prefix_list"`
	MatchNextHopPrefixList string        `yaml:"match_next_hop_prefix_list"`
	MatchASPathList        string        `yaml:"match_as_path_list"`
	MatchCommunityList     string        `yaml:"match_community_list"`
	MatchCommunityExact    bool          `yaml:"match_community_exact,omitempty"`
	SetLocalPref           *int          `yaml:"set_local_pref,omitempty"`
	SetLocalPrefDelta      *int          `yaml:"set_local_pref_delta,omitempty"`
	SetMED                 *int          `yaml:"set_med,omitempty"`
	SetMEDDelta            *int          `yaml:"set_med_delta,omitempty"`
	SetASPathPrepend       []uint32      `yaml:"set_as_path_prepend,omitempty"`
	SetCommunities         []string      `yaml:"set_communities,omitempty"`
	SetCommunityAdditive   bool          `yaml:"set_community_additive,omitempty"`
	SetOriginCode          BGPOriginCode `yaml:"set_origin_code,omitempty"`
	SetNextHop             string        `yaml:"set_next_hop,omitempty"`
	SetNextHopSelf         bool          `yaml:"set_next_hop_self,omitempty"`
}
