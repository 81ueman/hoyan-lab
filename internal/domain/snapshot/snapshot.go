package snapshot

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

const Version = "hoyan.live_snapshot.v1"

type Snapshot struct {
	Version      string                  `json:"version"`
	Lab          string                  `json:"lab,omitempty"`
	TopologyPath string                  `json:"topology_path,omitempty"`
	TopologyHash string                  `json:"topology_hash,omitempty"`
	ConfigHashes map[string]string       `json:"config_hashes,omitempty"`
	GitCommit    string                  `json:"git_commit,omitempty"`
	CollectedAt  time.Time               `json:"collected_at"`
	Nodes        map[string]NodeSnapshot `json:"nodes"`
	Warnings     []string                `json:"warnings,omitempty"`
}

type NodeSnapshot struct {
	Kind          model.DeviceKind              `json:"kind"`
	BGPRIB        []observation.RIBRoute        `json:"bgp_rib,omitempty"`
	RouteTable    []observation.RIBRoute        `json:"route_table,omitempty"`
	FIB           []observation.FIBEntry        `json:"fib,omitempty"`
	UnresolvedFIB []observation.UnresolvedRoute `json:"unresolved_fib,omitempty"`
	Raw           map[string]json.RawMessage    `json:"raw,omitempty"`
}

type HashPolicy string

const (
	HashPolicyWarn   HashPolicy = "warn"
	HashPolicyFail   HashPolicy = "fail"
	HashPolicyIgnore HashPolicy = "ignore"
)

type HashMismatch struct {
	Path string
	Want string
	Got  string
}

type HashCheckResult struct {
	Mismatches []HashMismatch
	Missing    []string
}

type InputHashSet struct {
	TopologyHash string
	ConfigHashes map[string]string
}

func ParseHashPolicy(raw string) (HashPolicy, bool) {
	switch HashPolicy(strings.ToLower(strings.TrimSpace(raw))) {
	case "", HashPolicyWarn:
		return HashPolicyWarn, true
	case HashPolicyFail:
		return HashPolicyFail, true
	case HashPolicyIgnore:
		return HashPolicyIgnore, true
	default:
		return HashPolicy(raw), false
	}
}
