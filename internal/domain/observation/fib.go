package observation

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

type FIB struct {
	Node    model.NodeID            `json:"node"`
	VRF     model.NetworkInstanceID `json:"vrf"`
	Entries []FIBEntry              `json:"entries"`
}

type FIBEntry struct {
	Node           string                    `json:"node,omitempty"`
	VRF            string                    `json:"vrf,omitempty"`
	Protocol       string                    `json:"protocol,omitempty"`
	ConnectedClass model.ConnectedRouteClass `json:"connected_class,omitempty"`
	Installed      bool                      `json:"installed,omitempty"`

	AFI    model.AFI `json:"afi"`
	Prefix string    `json:"prefix"`

	Source RouteSource      `json:"source"`
	Action ForwardingAction `json:"action"`

	NextHops []NextHop `json:"next_hops,omitempty"`

	Preference int `json:"preference,omitempty"`
	Metric     int `json:"metric,omitempty"`

	ModelInfo *ModelRouteInfo `json:"model_info,omitempty"`
}

type ForwardingAction string

const (
	ActionForward ForwardingAction = "forward"
	ActionDrop    ForwardingAction = "drop"
	ActionReceive ForwardingAction = "receive"
)

func (e FIBEntry) Validate() error {
	if e.Prefix == "" {
		return errors.New("fib entry prefix is required")
	}
	if NormalizeRouteProtocol(e.Source.Protocol) != e.Source.Protocol {
		return fmt.Errorf("fib entry source protocol %q is not normalized", e.Source.Protocol)
	}
	switch e.Action {
	case ActionForward, ActionDrop, ActionReceive:
		return nil
	default:
		return fmt.Errorf("fib entry action %q is invalid", e.Action)
	}
}

func (f FIB) Validate() error {
	if f.Node == "" {
		return errors.New("fib node is required")
	}
	if f.VRF == "" {
		return errors.New("fib vrf is required")
	}
	for i, entry := range f.Entries {
		if err := entry.Validate(); err != nil {
			return fmt.Errorf("fib entry %d: %w", i, err)
		}
	}
	return nil
}

func (f FIB) Key() string {
	return string(f.Node) + "|" + string(f.VRF)
}

func (e FIBEntry) Key() string {
	return strings.Join([]string{
		string(model.NormalizeAFI(e.AFI)),
		string(NormalizeRouteProtocol(e.Source.Protocol)),
		string(e.Action),
		e.Prefix,
	}, "|")
}

func SortFIBEntries(entries []FIBEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Key() < entries[j].Key()
	})
}

func FilterFIBEntries(entries []FIBEntry, pred func(FIBEntry) bool) []FIBEntry {
	out := make([]FIBEntry, 0, len(entries))
	for _, entry := range entries {
		if pred(entry) {
			out = append(out, entry)
		}
	}
	return out
}

func FIBFromRouteRecords(node model.NodeID, vrf model.NetworkInstanceID, routes []FIBEntry) FIB {
	requestedVRF := vrf
	out := FIB{Node: node, VRF: model.NormalizeNetworkInstance(string(vrf))}
	for _, route := range routes {
		if node == "" {
			out.Node = model.NodeID(route.Node)
		}
		if requestedVRF == "" {
			out.VRF = model.NormalizeNetworkInstance(route.VRF)
		}
		out.Entries = append(out.Entries, FIBEntryFromRouteRecord(route))
	}
	SortFIBEntries(out.Entries)
	return out
}

func FIBsFromRouteRecords(routes []FIBEntry) []FIB {
	byKey := map[string]*FIB{}
	for _, route := range routes {
		node := model.NodeID(route.Node)
		vrf := model.NormalizeNetworkInstance(route.VRF)
		key := string(node) + "|" + string(vrf)
		if byKey[key] == nil {
			byKey[key] = &FIB{Node: node, VRF: vrf}
		}
		byKey[key].Entries = append(byKey[key].Entries, FIBEntryFromRouteRecord(route))
	}
	out := make([]FIB, 0, len(byKey))
	for _, fib := range byKey {
		SortFIBEntries(fib.Entries)
		out = append(out, *fib)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Key() < out[j].Key()
	})
	return out
}

func FIBEntryFromRouteRecord(route FIBEntry) FIBEntry {
	protocol := NormalizeRouteProtocol(RouteProtocol(route.Protocol))
	action := ActionForward
	if protocol == ProtocolBlackhole {
		action = ActionDrop
	} else if protocol == ProtocolConnected && len(route.NextHops) == 0 {
		action = ActionReceive
	}
	return FIBEntry{
		AFI:    model.NormalizeAFI(model.AFI(route.AFI)),
		Prefix: route.Prefix,
		Source: RouteSource{
			Protocol: protocol,
		},
		Action:     action,
		NextHops:   nextHopsFromRouteRecordFIB(route.NextHops),
		Preference: route.Preference,
		Metric:     route.Metric,
	}
}

func nextHopsFromRouteRecordFIB(hops []NextHop) []NextHop {
	out := make([]NextHop, 0, len(hops))
	for _, hop := range hops {
		out = append(out, NextHop{
			Address:   hop.Address,
			Interface: hop.Interface,
			Weight:    hop.Weight,
		})
	}
	return out
}
