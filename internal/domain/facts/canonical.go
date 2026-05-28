package facts

import (
	"fmt"
	"sort"
)

type RIBRow struct {
	Snapshot  string `json:"snapshot"`
	Device    string `json:"device"`
	VRF       string `json:"vrf,omitempty"`
	Prefix    string `json:"prefix"`
	Protocol  string `json:"protocol,omitempty"`
	NextHop   string `json:"nexthop,omitempty"`
	LocalPref int    `json:"local_pref,omitempty"`
	MED       int    `json:"med,omitempty"`
	Selected  bool   `json:"selected"`
	Installed bool   `json:"installed,omitempty"`
}

type FIBRow struct {
	Snapshot  string `json:"snapshot"`
	Device    string `json:"device"`
	Prefix    string `json:"prefix"`
	NextHop   string `json:"nexthop,omitempty"`
	Interface string `json:"interface,omitempty"`
	Installed bool   `json:"installed"`
}

type CanonicalRIBRow struct {
	Device    string `json:"device"`
	VRF       string `json:"vrf,omitempty"`
	Prefix    string `json:"prefix"`
	Protocol  string `json:"protocol,omitempty"`
	NextHop   string `json:"nexthop,omitempty"`
	LocalPref int    `json:"local_pref,omitempty"`
	MED       int    `json:"med,omitempty"`
	Selected  bool   `json:"selected"`
	Installed bool   `json:"installed,omitempty"`
}

func CanonicalRIBRows(rows []RIBRow) []CanonicalRIBRow {
	out := make([]CanonicalRIBRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, CanonicalRIBRow{
			Device:    row.Device,
			VRF:       row.VRF,
			Prefix:    row.Prefix,
			Protocol:  row.Protocol,
			NextHop:   row.NextHop,
			LocalPref: row.LocalPref,
			MED:       row.MED,
			Selected:  row.Selected,
			Installed: row.Installed,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Key() < out[j].Key()
	})
	return out
}

func (row CanonicalRIBRow) Key() string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%010d\x00%010d\x00%t\x00%t",
		row.Device, row.VRF, row.Prefix, row.Protocol, row.NextHop, row.LocalPref, row.MED, row.Selected, row.Installed)
}
