package observation

import (
	"context"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

// CollectSnapshot orchestrates collection of RIB/FIB data from all nodes and VRFs.
//
// Deprecated: collection orchestration belongs to the collect usecase. New code
// should use internal/usecase/collect.CollectSnapshot.
func CollectSnapshot(ctx context.Context, collector Collector, opts CollectOptions) (NetworkSnapshot, error) {
	nodes, err := collector.Nodes(ctx)
	if err != nil {
		return NetworkSnapshot{}, err
	}
	sortNodeIDs(nodes)
	snapshot := NetworkSnapshot{Nodes: make([]NodeSnapshot, 0, len(nodes))}
	if provider, ok := collector.(MetadataProvider); ok {
		metadata := provider.Metadata(ctx)
		snapshot.Metadata.Source = metadata.Source
		snapshot.Metadata.Labels = cloneStringMap(metadata.Labels)
	}
	for _, node := range nodes {
		vrfs, err := collector.VRFs(ctx, node)
		if err != nil {
			return NetworkSnapshot{}, err
		}
		sortNetworkInstanceIDs(vrfs)
		nodeSnapshot := NodeSnapshot{Node: node, VRFs: make([]VRFSnapshot, 0, len(vrfs))}
		for _, vrf := range vrfs {
			rib, err := collector.CollectRIB(ctx, model.Node{Name: string(node)}, vrf, opts)
			if err != nil {
				return NetworkSnapshot{}, err
			}
			fib, err := collector.CollectFIB(ctx, model.Node{Name: string(node)}, vrf, Options{})
			if err != nil {
				return NetworkSnapshot{}, err
			}
			nodeSnapshot.VRFs = append(nodeSnapshot.VRFs, VRFSnapshot{
				VRF: vrf,
				RIB: normalizeRIBForSnapshot(node, vrf, rib, opts),
				FIB: normalizeFIBForSnapshot(node, vrf, fib, opts),
			})
		}
		snapshot.Nodes = append(snapshot.Nodes, nodeSnapshot)
	}
	return snapshot, nil
}
