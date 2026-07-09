package observation

import (
	"context"
	"fmt"
	"sort"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

type SnapshotBackedCollector struct {
	snapshot NetworkSnapshot
}

func NewSnapshotBackedCollector(snapshot NetworkSnapshot) SnapshotBackedCollector {
	return SnapshotBackedCollector{snapshot: NormalizeNetworkSnapshot(snapshot)}
}

func (c SnapshotBackedCollector) Metadata(context.Context) CollectorMetadata {
	return CollectorMetadata{
		Source: c.snapshot.Metadata.Source,
		Labels: cloneStringMap(c.snapshot.Metadata.Labels),
	}
}

func (c SnapshotBackedCollector) Nodes(context.Context) ([]model.NodeID, error) {
	out := make([]model.NodeID, 0, len(c.snapshot.Nodes))
	for _, node := range c.snapshot.Nodes {
		out = append(out, node.Node)
	}
	SortNodeIDs(out)
	return out, nil
}

func (c SnapshotBackedCollector) VRFs(_ context.Context, node model.NodeID) ([]model.NetworkInstanceID, error) {
	ns, ok := c.node(node)
	if !ok {
		return nil, fmt.Errorf("snapshot node %q not found", node)
	}
	out := make([]model.NetworkInstanceID, 0, len(ns.VRFs))
	for _, vrf := range ns.VRFs {
		out = append(out, vrf.VRF)
	}
	SortNetworkInstanceIDs(out)
	return out, nil
}

func (c SnapshotBackedCollector) CollectRIB(_ context.Context, node model.NodeID, vrf model.NetworkInstanceID, opts CollectOptions) (RIB, error) {
	vs, ok := c.vrf(node, vrf)
	if !ok {
		return RIB{}, fmt.Errorf("snapshot RIB %q/%q not found", node, vrf)
	}
	return normalizeRIBForSnapshot(node, vrf, vs.RIB, opts), nil
}

func (c SnapshotBackedCollector) CollectFIB(_ context.Context, node model.NodeID, vrf model.NetworkInstanceID, _ Options) (FIB, error) {
	vs, ok := c.vrf(node, vrf)
	if !ok {
		return FIB{}, fmt.Errorf("snapshot FIB %q/%q not found", node, vrf)
	}
	return normalizeFIBForSnapshot(node, vrf, vs.FIB, CollectOptions{IncludeModelInfo: true}), nil
}

func (c SnapshotBackedCollector) node(node model.NodeID) (NodeSnapshot, bool) {
	for _, ns := range c.snapshot.Nodes {
		if ns.Node == node {
			return ns, true
		}
	}
	return NodeSnapshot{}, false
}

func (c SnapshotBackedCollector) vrf(node model.NodeID, vrf model.NetworkInstanceID) (VRFSnapshot, bool) {
	ns, ok := c.node(node)
	if !ok {
		return VRFSnapshot{}, false
	}
	for _, vs := range ns.VRFs {
		if model.NormalizeNetworkInstance(string(vs.VRF)) == model.NormalizeNetworkInstance(string(vrf)) {
			return vs, true
		}
	}
	return VRFSnapshot{}, false
}

func NormalizeNetworkSnapshot(snapshot NetworkSnapshot) NetworkSnapshot {
	out := NetworkSnapshot{
		Metadata: SnapshotMetadata{
			ID:          snapshot.Metadata.ID,
			Source:      snapshot.Metadata.Source,
			CollectedAt: snapshot.Metadata.CollectedAt,
			Labels:      cloneStringMap(snapshot.Metadata.Labels),
		},
		Nodes: make([]NodeSnapshot, 0, len(snapshot.Nodes)),
	}
	for _, ns := range snapshot.Nodes {
		node := NodeSnapshot{Node: ns.Node, VRFs: make([]VRFSnapshot, 0, len(ns.VRFs))}
		for _, vs := range ns.VRFs {
			vrf := model.NormalizeNetworkInstance(string(vs.VRF))
			rib := normalizeRIBForSnapshot(ns.Node, vrf, vs.RIB, CollectOptions{IncludeInactive: true, IncludeModelInfo: true})
			fib := normalizeFIBForSnapshot(ns.Node, vrf, vs.FIB, CollectOptions{IncludeModelInfo: true})
			node.VRFs = append(node.VRFs, VRFSnapshot{VRF: vrf, RIB: rib, FIB: fib})
		}
		sort.SliceStable(node.VRFs, func(i, j int) bool {
			return node.VRFs[i].VRF < node.VRFs[j].VRF
		})
		out.Nodes = append(out.Nodes, node)
	}
	sort.SliceStable(out.Nodes, func(i, j int) bool {
		return out.Nodes[i].Node < out.Nodes[j].Node
	})
	// migrateSnapshotForCompare fills in missing canonical fields (AFI, Action)
	// that older snapshots may lack. This is the file-load migration boundary;
	// compare-time code no longer defaults these fields.
	out = migrateSnapshotForCompare(out)
	return out
}

// migrateSnapshotForCompare fills in empty AFI/Action on RIB routes and FIB
// entries loaded from old snapshots. New producers must emit explicit
// normalized values; compare-time defaults have been removed.
// This migration runs at the file-load boundary only (snapshotfile.Load,
// NormalizeNetworkSnapshot) and is documented here as a one-time compat layer.
func migrateSnapshotForCompare(snapshot NetworkSnapshot) NetworkSnapshot {
	for ni := range snapshot.Nodes {
		for vi := range snapshot.Nodes[ni].VRFs {
			for ri := range snapshot.Nodes[ni].VRFs[vi].RIB.Routes {
				route := &snapshot.Nodes[ni].VRFs[vi].RIB.Routes[ri]
				if route.Common.AFI == "" {
					route.Common.AFI = model.AFIIPv4
				}
			}
			for fi := range snapshot.Nodes[ni].VRFs[vi].FIB.Entries {
				entry := &snapshot.Nodes[ni].VRFs[vi].FIB.Entries[fi]
				if entry.AFI == "" {
					entry.AFI = model.AFIIPv4
				}
				if entry.Action == "" {
					if entry.Source.Protocol == model.RouteSourceBlackhole {
						entry.Action = ActionDrop
					} else if entry.Source.Protocol == model.RouteSourceConnected && len(entry.NextHops) == 0 {
						entry.Action = ActionReceive
					} else {
						entry.Action = ActionForward
					}
				}
			}
		}
	}
	return snapshot
}
