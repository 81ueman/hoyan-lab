package controlplane

import (
	"net/netip"
	"sort"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func (e *Engine) ospfVRFs() []model.NetworkInstanceID {
	seen := map[model.NetworkInstanceID]bool{}
	for _, node := range e.idx.Topology.Nodes {
		for _, process := range ospfProcessesForNode(node) {
			if !process.Enabled {
				continue
			}
			vrf := model.NormalizeNetworkInstance(string(process.NetworkInstance))
			seen[vrf] = true
		}
	}
	out := make([]model.NetworkInstanceID, 0, len(seen))
	for vrf := range seen {
		out = append(out, vrf)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (e *Engine) ospfProcesses(vrf model.NetworkInstanceID) map[string]model.OSPFProcess {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	out := map[string]model.OSPFProcess{}
	for _, node := range e.idx.Topology.Nodes {
		for _, process := range ospfProcessesForNode(node) {
			if !process.Enabled || model.NormalizeNetworkInstance(string(process.NetworkInstance)) != vrf {
				continue
			}
			process.NetworkInstance = vrf
			out[node.Name] = process
		}
	}
	return out
}

func ospfProcessesForNode(node model.Node) []model.OSPFProcess {
	var out []model.OSPFProcess
	if node.OSPF.Enabled {
		process := node.OSPF
		process.NetworkInstance = model.NormalizeNetworkInstance(string(process.NetworkInstance))
		out = append(out, process)
	}
	for _, process := range node.OSPFProcesses {
		if !process.Enabled {
			continue
		}
		process.NetworkInstance = model.NormalizeNetworkInstance(string(process.NetworkInstance))
		out = append(out, process)
	}
	return out
}

func ospfInterfaceForPrefix(states map[string]InterfaceState, prefix model.Prefix) string {
	for _, state := range states {
		if model.PrefixFromNetIP(state.Prefix).Equal(prefix) {
			return state.Name
		}
	}
	return ""
}

func (e *Engine) ospfInterfaceStates(vrf model.NetworkInstanceID, processes map[string]model.OSPFProcess) map[string]map[string]InterfaceState {
	out := map[string]map[string]InterfaceState{}
	for _, node := range e.idx.Topology.Nodes {
		process, ok := processes[node.Name]
		if !ok {
			continue
		}
		for _, iface := range node.Interfaces {
			pfx, err := netip.ParsePrefix(iface.Address)
			if err != nil || !pfx.Addr().Is4() {
				continue
			}
			ifState, ok := ospfInterfaceFor(node, process, vrf, iface, pfx)
			if !ok {
				continue
			}
			if out[node.Name] == nil {
				out[node.Name] = map[string]InterfaceState{}
			}
			out[node.Name][iface.Name] = ifState
		}
	}
	return out
}

func ospfInterfaceFor(node model.Node, process model.OSPFProcess, vrf model.NetworkInstanceID, iface model.Interface, pfx netip.Prefix) (InterfaceState, bool) {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	if model.NormalizeNetworkInstance(string(iface.VRF)) != vrf {
		return InterfaceState{}, false
	}
	state := InterfaceState{Node: node.Name, Name: iface.Name, NetworkInstance: vrf, Prefix: pfx.Masked(), Cost: 1}
	for _, configured := range process.Interfaces {
		if !model.EquivalentInterfaceName(node.Kind, configured.Name, iface.Name) {
			continue
		}
		state.Area = NormalizeArea(configured.Area)
		if configured.Cost > 0 {
			state.Cost = configured.Cost
		}
		state.Passive = configured.Passive
		state.NetworkType = configured.NetworkType
	}
	if state.Area == "" {
		for _, network := range process.Networks {
			if network.Prefix.Contains(pfx.Addr()) {
				state.Area = NormalizeArea(network.Area)
				break
			}
		}
	}
	for _, passive := range process.PassiveInterfaces {
		if model.EquivalentInterfaceName(node.Kind, passive, iface.Name) {
			state.Passive = true
		}
	}
	if state.Area == "" {
		return InterfaceState{}, false
	}
	if IsLoopbackInterface(iface.Name) {
		state.Cost = 0
	}
	return state, true
}

func (e *Engine) ospfAdvertisements(states map[string]map[string]InterfaceState, processes map[string]model.OSPFProcess) []Advertisement {
	var out []Advertisement
	seen := map[string]bool{}
	for node, byIface := range states {
		for _, state := range byIface {
			prefix := model.PrefixFromNetIP(state.Prefix)
			key := node + "|" + string(state.NetworkInstance) + "|" + prefix.String()
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, Advertisement{Node: node, NetworkInstance: state.NetworkInstance, Prefix: prefix, Cost: state.Cost, Area: state.Area})
		}
	}
	for _, node := range e.idx.Topology.Nodes {
		process, ok := processes[node.Name]
		if !ok {
			continue
		}
		for _, route := range e.ospfRedistributedRoutes(node, process) {
			area := ExternalArea(process, states[node.Name])
			out = append(out, Advertisement{
				Node:            node.Name,
				NetworkInstance: process.NetworkInstance,
				Prefix:          route.RouteSource.Prefix,
				Cost:            route.RouteSource.Metric,
				External:        true,
				MetricType:      route.RouteSource.MetricType,
				ExternalArea:    area,
				Source:          route,
			})
		}
		for _, area := range process.Areas {
			if area.Kind == model.OSPFAreaStub && !NodeAttachedToOtherArea(states[node.Name], area.ID) {
				continue
			}
			if area.Kind != model.OSPFAreaStub && !(area.Kind == model.OSPFAreaNSSA && area.DefaultInformationOriginate) {
				continue
			}
			if !NodeAttachedToArea(states[node.Name], area.ID) {
				continue
			}
			out = append(out, Advertisement{Node: node.Name, NetworkInstance: process.NetworkInstance, Prefix: model.MustPrefix("0.0.0.0/0"), Cost: 1, DefaultArea: area.ID})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Node == out[j].Node {
			return out[i].Prefix.String() < out[j].Prefix.String()
		}
		return out[i].Node < out[j].Node
	})
	return out
}
