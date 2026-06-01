package snapshot

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

const Version = "hoyan.live_snapshot.v1"

type Snapshot struct {
	Version      string                      `json:"version"`
	Lab          string                      `json:"lab,omitempty"`
	TopologyPath string                      `json:"topology_path,omitempty"`
	TopologyHash string                      `json:"topology_hash,omitempty"`
	ConfigHashes map[string]string           `json:"config_hashes,omitempty"`
	GitCommit    string                      `json:"git_commit,omitempty"`
	CollectedAt  time.Time                   `json:"collected_at"`
	Nodes        map[string]NodeSnapshot     `json:"nodes"`
	Network      observation.NetworkSnapshot `json:"network"`
	Warnings     []string                    `json:"warnings,omitempty"`
}

type NodeSnapshot struct {
	Kind model.DeviceKind           `json:"kind"`
	Raw  map[string]json.RawMessage `json:"raw,omitempty"`
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

func BGPRoutes(snap *Snapshot) []observation.RIBRoute {
	return observation.BGPOnly(RIBRoutes(snap))
}

func RIBRoutes(snap *Snapshot) []observation.RIBRoute {
	var out []observation.RIBRoute
	for _, node := range sortedObservationNodes(snap.Network.Nodes) {
		for _, vrf := range sortedObservationVRFs(node.VRFs) {
			out = append(out, vrf.RIB.Routes...)
		}
	}
	observation.SortRoutes(out)
	return out
}

func FIBRoutes(snap *Snapshot) []observation.FIBEntry {
	var out []observation.FIBEntry
	for _, fib := range FIBs(snap) {
		out = append(out, fib.Entries...)
	}
	return out
}

func FIBs(snap *Snapshot) []observation.FIB {
	var out []observation.FIB
	for _, node := range sortedObservationNodes(snap.Network.Nodes) {
		for _, vrf := range sortedObservationVRFs(node.VRFs) {
			out = append(out, vrf.FIB)
		}
	}
	return out
}

func sortedObservationNodes(nodes []observation.NodeSnapshot) []observation.NodeSnapshot {
	out := append([]observation.NodeSnapshot(nil), nodes...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Node < out[j].Node })
	return out
}

func sortedObservationVRFs(vrfs []observation.VRFSnapshot) []observation.VRFSnapshot {
	out := append([]observation.VRFSnapshot(nil), vrfs...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].VRF < out[j].VRF })
	return out
}
