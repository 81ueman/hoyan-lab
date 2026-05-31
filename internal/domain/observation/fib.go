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
}

type ForwardingAction string

const (
	ActionForward ForwardingAction = "forward"
	ActionDrop    ForwardingAction = "drop"
	ActionReceive ForwardingAction = "receive"
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
	case ActionDrop, ActionReceive:
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
