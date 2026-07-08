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
	AFI    model.AFI `json:"afi"`
	Prefix string    `json:"prefix"`

	Source RouteSource      `json:"source"`
	Action ForwardingAction `json:"action"`

	NextHops []NextHop `json:"next_hops,omitempty"`

	Preference int `json:"preference,omitempty"`
	Metric     int `json:"metric,omitempty"`

	ModelInfo *ModelRouteInfo `json:"model_info,omitempty"`

	// SAFI is the Subsequent Address Family Identifier (e.g. "unicast",
	// "multicast", "vpn-unicast"). Zero value means unknown / not reported.
	SAFI string `json:"safi,omitempty"`
	// TableID is the routing table identifier as reported by the device.
	TableID string `json:"table_id,omitempty"`
	// TableName is a human-friendly name for the routing table.
	TableName string `json:"table_name,omitempty"`
	// ProtocolInstance identifies the specific protocol process or
	// instance (e.g. "BGP 65000", "OSPF 100", "OSPFv3 1").
	ProtocolInstance string `json:"protocol_instance,omitempty"`
	// Age is the route age as a human-readable string (e.g. "00:12:34",
	// "1h2m3s").
	Age string `json:"age,omitempty"`
	// AgeSeconds is the route age in seconds.
	AgeSeconds int `json:"age_seconds,omitempty"`
	// Tag is an administratively assigned route tag value.
	Tag uint32 `json:"tag,omitempty"`
	// InstalledReason describes why the route was or was not installed
	// into the FIB (e.g. "active", "inactive", "fib", "not_selected").
	InstalledReason string `json:"installed_reason,omitempty"`
	// Raw holds vendor-specific attributes that do not have a dedicated
	// field in this schema. It is never compared by default.
	Raw map[string]any `json:"raw,omitempty"`
}

type ForwardingAction string

const (
	ActionForward ForwardingAction = "forward"
	ActionDrop    ForwardingAction = "drop"
	ActionReceive ForwardingAction = "receive"
	ActionReject  ForwardingAction = "reject"
	ActionUnreach ForwardingAction = "unreachable"
	ActionPunt    ForwardingAction = "punt"
	ActionTrap    ForwardingAction = "trap"
	ActionLocal   ForwardingAction = "local"
	ActionGlean   ForwardingAction = "glean"
	ActionDiscard ForwardingAction = "discard"
)

func (e FIBEntry) Validate() error {
	if model.NormalizeAFI(e.AFI) != e.AFI {
		return fmt.Errorf("fib entry afi %q is not normalized", e.AFI)
	}
	if e.Prefix == "" {
		return errors.New("fib entry prefix is required")
	}
	if model.NormalizeRouteSourceKind(e.Source.Protocol) != e.Source.Protocol {
		return fmt.Errorf("fib entry source protocol %q is not normalized", e.Source.Protocol)
	}
	switch e.Action {
	case ActionForward:
		if len(e.NextHops) == 0 {
			return errors.New("fib forward entry requires at least one next-hop")
		}
	case ActionDrop, ActionReceive, ActionReject, ActionUnreach, ActionPunt, ActionTrap, ActionLocal, ActionGlean, ActionDiscard:
	default:
		return fmt.Errorf("fib entry action %q is invalid", e.Action)
	}
	return nil
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
		string(model.NormalizeRouteSourceKind(e.Source.Protocol)),
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
