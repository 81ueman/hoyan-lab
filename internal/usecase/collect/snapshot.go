package collect

import (
	"context"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

// RIBCollector is the usecase-local port for collecting RIB data, keyed by node identity.
type RIBCollector interface {
	CollectRIB(ctx context.Context, node model.NodeID, vrf model.NetworkInstanceID, opts observation.CollectOptions) (observation.RIB, error)
}

// FIBCollector is the usecase-local port for collecting FIB data, keyed by node identity.
type FIBCollector interface {
	CollectFIB(ctx context.Context, node model.NodeID, vrf model.NetworkInstanceID, opts observation.Options) (observation.FIB, error)
}

// Collector is the port required by the snapshot collection usecase.
type Collector interface {
	RIBCollector
	FIBCollector

	Nodes(ctx context.Context) ([]model.NodeID, error)
	VRFs(ctx context.Context, node model.NodeID) ([]model.NetworkInstanceID, error)
}

// MetadataProvider optionally supplies metadata for collected snapshots.
type MetadataProvider interface {
	Metadata(ctx context.Context) observation.CollectorMetadata
}

// CollectSnapshot orchestrates collection of RIB/FIB data from all nodes and VRFs.
func CollectSnapshot(ctx context.Context, collector Collector, opts observation.CollectOptions) (observation.NetworkSnapshot, error) {
	nodes, err := collector.Nodes(ctx)
	if err != nil {
		return observation.NetworkSnapshot{}, err
	}
	observation.SortNodeIDs(nodes)
	snapshot := observation.NetworkSnapshot{Nodes: make([]observation.NodeSnapshot, 0, len(nodes))}
	if provider, ok := collector.(MetadataProvider); ok {
		metadata := provider.Metadata(ctx)
		snapshot.Metadata.Source = metadata.Source
		snapshot.Metadata.Labels = cloneStringMap(metadata.Labels)
	}
	for _, node := range nodes {
		vrfs, err := collector.VRFs(ctx, node)
		if err != nil {
			return observation.NetworkSnapshot{}, err
		}
		observation.SortNetworkInstanceIDs(vrfs)
		nodeSnapshot := observation.NodeSnapshot{Node: node, VRFs: make([]observation.VRFSnapshot, 0, len(vrfs))}
		for _, vrf := range vrfs {
			rib, err := collector.CollectRIB(ctx, node, vrf, opts)
			if err != nil {
				return observation.NetworkSnapshot{}, err
			}
			fib, err := collector.CollectFIB(ctx, node, vrf, observation.Options{AFI: opts.AFI})
			if err != nil {
				return observation.NetworkSnapshot{}, err
			}
			nodeSnapshot.VRFs = append(nodeSnapshot.VRFs, observation.VRFSnapshot{
				VRF: vrf,
				RIB: normalizeRIBForSnapshot(node, vrf, rib, opts),
				FIB: normalizeFIBForSnapshot(node, vrf, fib, opts),
			})
		}
		snapshot.Nodes = append(snapshot.Nodes, nodeSnapshot)
	}
	return snapshot, nil
}

// CompareCollectors collects snapshots from two collectors and compares them.
func CompareCollectors(ctx context.Context, expected, actual Collector, collectOpts observation.CollectOptions, compareOpts observation.SnapshotCompareOptions) (observation.SnapshotComparison, error) {
	expectedSnapshot, err := CollectSnapshot(ctx, expected, collectOpts)
	if err != nil {
		return observation.SnapshotComparison{}, err
	}
	actualSnapshot, err := CollectSnapshot(ctx, actual, collectOpts)
	if err != nil {
		return observation.SnapshotComparison{}, err
	}
	return observation.CompareSnapshots(expectedSnapshot, actualSnapshot, compareOpts), nil
}

func normalizeRIBForSnapshot(node model.NodeID, vrf model.NetworkInstanceID, rib observation.RIB, opts observation.CollectOptions) observation.RIB {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	rib.Node = node
	rib.VRF = vrf
	return observation.FilterRIB(rib, opts)
}

func normalizeFIBForSnapshot(node model.NodeID, vrf model.NetworkInstanceID, fib observation.FIB, opts observation.CollectOptions) observation.FIB {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	fib.Node = node
	fib.VRF = vrf
	return observation.FilterFIB(fib, opts)
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
