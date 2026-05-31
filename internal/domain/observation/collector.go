package observation

import (
	"context"
	"net/netip"
	"sort"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

type Collector interface {
	Nodes(ctx context.Context) ([]model.NodeID, error)
	VRFs(ctx context.Context, node model.NodeID) ([]VRFName, error)

	CollectRIB(ctx context.Context, node model.NodeID, vrf VRFName, opts CollectOptions) (RIB, error)
	CollectFIB(ctx context.Context, node model.NodeID, vrf VRFName, opts CollectOptions) (FIB, error)
}

type CollectOptions struct {
	AFI       AddressFamily
	Protocols []RouteProtocol
	Prefixes  []netip.Prefix

	IncludeInactive  bool
	IncludeModelInfo bool
}

type CollectorMetadata struct {
	Source string            `json:"source,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
}

type MetadataProvider interface {
	Metadata(ctx context.Context) CollectorMetadata
}

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
		sortVRFNames(vrfs)
		nodeSnapshot := NodeSnapshot{Node: node, VRFs: make([]VRFSnapshot, 0, len(vrfs))}
		for _, vrf := range vrfs {
			rib, err := collector.CollectRIB(ctx, node, vrf, opts)
			if err != nil {
				return NetworkSnapshot{}, err
			}
			fib, err := collector.CollectFIB(ctx, node, vrf, opts)
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

func FilterRIB(route RIB, opts CollectOptions) RIB {
	out := RIB{Node: route.Node, VRF: route.VRF}
	for _, r := range route.Routes {
		if !opts.IncludeInactive && !r.Common.Best {
			continue
		}
		if !collectOptionsMatchRoute(opts, r.Common.AFI, r.Common.Protocol, r.Common.Prefix) {
			continue
		}
		if !opts.IncludeModelInfo {
			r.ModelInfo = nil
		}
		out.Routes = append(out.Routes, r)
	}
	SortRIBRoutes(out.Routes)
	return out
}

func FilterFIB(fib FIB, opts CollectOptions) FIB {
	out := FIB{Node: fib.Node, VRF: fib.VRF}
	for _, entry := range fib.Entries {
		if !collectOptionsMatchRoute(opts, entry.AFI, entry.Source.Protocol, entry.Prefix) {
			continue
		}
		if !opts.IncludeModelInfo {
			entry.ModelInfo = nil
		}
		out.Entries = append(out.Entries, entry)
	}
	SortFIBEntries(out.Entries)
	return out
}

func collectOptionsMatchRoute(opts CollectOptions, afi AddressFamily, protocol RouteProtocol, prefix string) bool {
	if opts.AFI != "" && NormalizeAddressFamily(opts.AFI) != NormalizeAddressFamily(afi) {
		return false
	}
	if len(opts.Protocols) > 0 {
		found := false
		for _, allowed := range opts.Protocols {
			if NormalizeRouteProtocol(allowed) == NormalizeRouteProtocol(protocol) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(opts.Prefixes) > 0 {
		parsed, err := netip.ParsePrefix(prefix)
		if err != nil {
			return false
		}
		found := false
		for _, allowed := range opts.Prefixes {
			if parsed == allowed {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func normalizeRIBForSnapshot(node model.NodeID, vrf VRFName, rib RIB, opts CollectOptions) RIB {
	rib.Node = node
	rib.VRF = vrf
	return FilterRIB(rib, opts)
}

func normalizeFIBForSnapshot(node model.NodeID, vrf VRFName, fib FIB, opts CollectOptions) FIB {
	fib.Node = node
	fib.VRF = vrf
	return FilterFIB(fib, opts)
}

func sortNodeIDs(nodes []model.NodeID) {
	sort.SliceStable(nodes, func(i, j int) bool {
		return nodes[i] < nodes[j]
	})
}

func sortVRFNames(vrfs []VRFName) {
	sort.SliceStable(vrfs, func(i, j int) bool {
		return vrfs[i] < vrfs[j]
	})
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
