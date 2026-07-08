package observation

import (
	"net/netip"
	"sort"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

type CollectOptions struct {
	AFI       model.AFI
	Protocols []model.RouteSourceKind
	Prefixes  []netip.Prefix

	IncludeInactive  bool
	IncludeModelInfo bool
}

type CollectorMetadata struct {
	Source string            `json:"source,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
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

func collectOptionsMatchRoute(opts CollectOptions, afi model.AFI, protocol model.RouteSourceKind, prefix string) bool {
	if opts.AFI != "" && model.NormalizeAFI(opts.AFI) != model.NormalizeAFI(afi) {
		return false
	}
	if len(opts.Protocols) > 0 {
		found := false
		for _, allowed := range opts.Protocols {
			if model.NormalizeRouteSourceKind(allowed) == model.NormalizeRouteSourceKind(protocol) {
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

func normalizeRIBForSnapshot(node model.NodeID, vrf model.NetworkInstanceID, rib RIB, opts CollectOptions) RIB {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	rib.Node = node
	rib.VRF = vrf
	return FilterRIB(rib, opts)
}

func normalizeFIBForSnapshot(node model.NodeID, vrf model.NetworkInstanceID, fib FIB, opts CollectOptions) FIB {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	fib.Node = node
	fib.VRF = vrf
	return FilterFIB(fib, opts)
}

func sortNodeIDs(nodes []model.NodeID) {
	sort.SliceStable(nodes, func(i, j int) bool {
		return nodes[i] < nodes[j]
	})
}

func sortNetworkInstanceIDs(vrfs []model.NetworkInstanceID) {
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
