package observation

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

type SnapshotCompareOptions struct {
	IgnoreMetadata     bool
	IgnoreModelInfo    bool
	IgnoreOptionalMeta bool // when true, strips new optional metadata fields
	// (SAFI, TableID, TableName, ProtocolInstance, Age,
	// AgeSeconds, Tag, InstalledReason, Raw, Resolved)
	// before comparing, so a snapshot that has them
	// populated does not cause noisy diffs against one
	// that does not.
}

type SnapshotComparison struct {
	OK              bool
	MissingNodes    []model.NodeID
	UnexpectedNodes []model.NodeID
	MissingVRFs     []SnapshotVRFKey
	UnexpectedVRFs  []SnapshotVRFKey
	RIBMismatches   []SnapshotTableMismatch
	FIBMismatches   []SnapshotTableMismatch
}

type SnapshotVRFKey struct {
	Node model.NodeID            `json:"node"`
	VRF  model.NetworkInstanceID `json:"vrf"`
}

type SnapshotTableMismatch struct {
	Node     model.NodeID            `json:"node"`
	VRF      model.NetworkInstanceID `json:"vrf"`
	Expected string                  `json:"expected"`
	Actual   string                  `json:"actual"`
}

// CompareCollectors collects snapshots from two collectors and compares them.
//
// Deprecated: collection orchestration belongs to the collect usecase. New code
// should use internal/usecase/collect.CompareCollectors.
func CompareCollectors(ctx context.Context, expected, actual Collector, collectOpts CollectOptions, compareOpts SnapshotCompareOptions) (SnapshotComparison, error) {
	expectedSnapshot, err := CollectSnapshot(ctx, expected, collectOpts)
	if err != nil {
		return SnapshotComparison{}, err
	}
	actualSnapshot, err := CollectSnapshot(ctx, actual, collectOpts)
	if err != nil {
		return SnapshotComparison{}, err
	}
	return CompareSnapshots(expectedSnapshot, actualSnapshot, compareOpts), nil
}

func CompareSnapshots(expected, actual NetworkSnapshot, opts SnapshotCompareOptions) SnapshotComparison {
	expected = snapshotForCompare(expected, opts)
	actual = snapshotForCompare(actual, opts)
	result := SnapshotComparison{}
	expectedNodes := snapshotNodesByID(expected)
	actualNodes := snapshotNodesByID(actual)
	for _, node := range sortedNodeMapKeys(expectedNodes, actualNodes) {
		expNode, expOK := expectedNodes[node]
		actNode, actOK := actualNodes[node]
		switch {
		case !expOK:
			result.UnexpectedNodes = append(result.UnexpectedNodes, node)
			continue
		case !actOK:
			result.MissingNodes = append(result.MissingNodes, node)
			continue
		}
		compareNodeSnapshot(expNode, actNode, &result)
	}
	result.OK = len(result.MissingNodes) == 0 &&
		len(result.UnexpectedNodes) == 0 &&
		len(result.MissingVRFs) == 0 &&
		len(result.UnexpectedVRFs) == 0 &&
		len(result.RIBMismatches) == 0 &&
		len(result.FIBMismatches) == 0
	return result
}

func compareNodeSnapshot(expected, actual NodeSnapshot, result *SnapshotComparison) {
	expectedVRFs := snapshotVRFsByName(expected)
	actualVRFs := snapshotVRFsByName(actual)
	for _, vrf := range sortedVRFMapKeys(expectedVRFs, actualVRFs) {
		expVRF, expOK := expectedVRFs[vrf]
		actVRF, actOK := actualVRFs[vrf]
		key := SnapshotVRFKey{Node: expected.Node, VRF: vrf}
		switch {
		case !expOK:
			result.UnexpectedVRFs = append(result.UnexpectedVRFs, key)
			continue
		case !actOK:
			result.MissingVRFs = append(result.MissingVRFs, key)
			continue
		}
		if !reflect.DeepEqual(expVRF.RIB, actVRF.RIB) {
			result.RIBMismatches = append(result.RIBMismatches, SnapshotTableMismatch{
				Node: expected.Node, VRF: vrf, Expected: stableJSON(expVRF.RIB), Actual: stableJSON(actVRF.RIB),
			})
		}
		if !reflect.DeepEqual(expVRF.FIB, actVRF.FIB) {
			result.FIBMismatches = append(result.FIBMismatches, SnapshotTableMismatch{
				Node: expected.Node, VRF: vrf, Expected: stableJSON(expVRF.FIB), Actual: stableJSON(actVRF.FIB),
			})
		}
	}
}

func snapshotForCompare(snapshot NetworkSnapshot, opts SnapshotCompareOptions) NetworkSnapshot {
	snapshot = NormalizeNetworkSnapshot(snapshot)
	if opts.IgnoreMetadata {
		snapshot.Metadata = SnapshotMetadata{}
	}
	if opts.IgnoreModelInfo {
		for ni := range snapshot.Nodes {
			for vi := range snapshot.Nodes[ni].VRFs {
				for ri := range snapshot.Nodes[ni].VRFs[vi].RIB.Routes {
					snapshot.Nodes[ni].VRFs[vi].RIB.Routes[ri].ModelInfo = nil
				}
				for fi := range snapshot.Nodes[ni].VRFs[vi].FIB.Entries {
					snapshot.Nodes[ni].VRFs[vi].FIB.Entries[fi].ModelInfo = nil
				}
			}
		}
	}
	if opts.IgnoreOptionalMeta {
		for ni := range snapshot.Nodes {
			for vi := range snapshot.Nodes[ni].VRFs {
				for ri := range snapshot.Nodes[ni].VRFs[vi].RIB.Routes {
					c := &snapshot.Nodes[ni].VRFs[vi].RIB.Routes[ri].Common
					c.SAFI = ""
					c.TableID = ""
					c.TableName = ""
					c.ProtocolInstance = ""
					c.Age = ""
					c.AgeSeconds = 0
					c.Tag = 0
					c.InstalledReason = ""
					c.Raw = nil
					if bgp := snapshot.Nodes[ni].VRFs[vi].RIB.Routes[ri].BGP; bgp != nil {
						for pi := range bgp.Paths {
							bgp.Paths[pi].Raw = nil
						}
					}
				}
				for fi := range snapshot.Nodes[ni].VRFs[vi].FIB.Entries {
					e := &snapshot.Nodes[ni].VRFs[vi].FIB.Entries[fi]
					e.SAFI = ""
					e.TableID = ""
					e.TableName = ""
					e.ProtocolInstance = ""
					e.Age = ""
					e.AgeSeconds = 0
					e.Tag = 0
					e.InstalledReason = ""
					e.Raw = nil
					for hi := range e.NextHops {
						e.NextHops[hi].Resolved = nil
						e.NextHops[hi].Raw = nil
					}
				}
			}
		}
	}
	return snapshot
}

func snapshotNodesByID(snapshot NetworkSnapshot) map[model.NodeID]NodeSnapshot {
	out := map[model.NodeID]NodeSnapshot{}
	for _, node := range snapshot.Nodes {
		out[node.Node] = node
	}
	return out
}

func snapshotVRFsByName(node NodeSnapshot) map[model.NetworkInstanceID]VRFSnapshot {
	out := map[model.NetworkInstanceID]VRFSnapshot{}
	for _, vrf := range node.VRFs {
		out[vrf.VRF] = vrf
	}
	return out
}

func sortedNodeMapKeys(a, b map[model.NodeID]NodeSnapshot) []model.NodeID {
	seen := map[model.NodeID]bool{}
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	out := make([]model.NodeID, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sortNodeIDs(out)
	return out
}

func sortedVRFMapKeys(a, b map[model.NetworkInstanceID]VRFSnapshot) []model.NetworkInstanceID {
	seen := map[model.NetworkInstanceID]bool{}
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	out := make([]model.NetworkInstanceID, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sortNetworkInstanceIDs(out)
	return out
}

func stableJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(data)
}
