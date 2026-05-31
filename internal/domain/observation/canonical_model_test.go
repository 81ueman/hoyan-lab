package observation

import (
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func TestRIBRouteValidateRequiresExactlyOneMatchingPayload(t *testing.T) {
	valid := RIBRoute{
		Common: RIBRouteCommon{AFI: model.AFIIPv4, Prefix: "10.0.0.0/24", Protocol: model.RouteSourceBGP, Eligible: true, Best: true},
		BGP:    &BGPRIBRoute{Paths: []BGPPath{{NextHop: NextHop{Address: "192.0.2.1"}, Eligible: true, Best: true}}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid BGP route failed validation: %v", err)
	}

	none := RIBRoute{Common: RIBRouteCommon{AFI: model.AFIIPv4, Prefix: "10.0.0.0/24", Protocol: model.RouteSourceBGP}}
	if err := none.Validate(); err == nil {
		t.Fatalf("route without protocol payload passed validation")
	}

	mismatch := RIBRoute{
		Common: RIBRouteCommon{AFI: model.AFIIPv4, Prefix: "10.0.0.0/24", Protocol: model.RouteSourceStatic},
		BGP:    &BGPRIBRoute{},
	}
	if err := mismatch.Validate(); err == nil {
		t.Fatalf("route with mismatched payload passed validation")
	}

	multiple := valid
	multiple.Static = &StaticRIBRoute{}
	if err := multiple.Validate(); err == nil {
		t.Fatalf("route with multiple payloads passed validation")
	}
}

func TestRIBsFromRouteRecordsGroupsAndConvertsBGP(t *testing.T) {
	routes := []RIBRoute{
		{
			Node:            "r2",
			NetworkInstance: "blue",
			Prefix:          "10.2.0.0/24",
			Paths:           []RIBPath{{Best: true, Valid: true, NextHop: "192.0.2.2", ASPath: []uint32{65002}, LocalPref: 200}},
		},
		{
			Node:            "r1",
			NetworkInstance: "default",
			AFI:             "ipv4",
			Prefix:          "10.1.0.0/24",
			Protocol:        "bgp",
			Paths:           []RIBPath{{Best: true, Valid: true, NextHop: "192.0.2.1", ASPath: []uint32{65001}, LocalPref: 100}},
		},
	}

	got := RIBsFromRouteRecords(routes)
	if len(got) != 2 {
		t.Fatalf("RIB count = %d, want 2", len(got))
	}
	if got[0].Node != "r1" || got[0].VRF != "default" {
		t.Fatalf("first RIB = %s/%s, want r1/default", got[0].Node, got[0].VRF)
	}
	route := got[0].Routes[0]
	if err := route.Validate(); err != nil {
		t.Fatalf("converted route failed validation: %v", err)
	}
	if route.BGP == nil || len(route.BGP.Paths) != 1 {
		t.Fatalf("converted route BGP payload = %#v", route.BGP)
	}
	if route.BGP.Paths[0].NextHop.Address != "192.0.2.1" || route.BGP.Paths[0].LocalPref != 100 {
		t.Fatalf("converted BGP path = %#v", route.BGP.Paths[0])
	}
}

func TestFIBEntryFromRouteRecordMapsForwardingAction(t *testing.T) {
	blackhole := FIBEntryFromRouteRecord(FIBEntry{
		AFI:       "ipv4",
		Prefix:    "203.0.113.0/24",
		Protocol:  "blackhole",
		Installed: true,
	})
	if blackhole.Source.Protocol != model.RouteSourceBlackhole || blackhole.Action != ActionDrop {
		t.Fatalf("blackhole conversion = %#v", blackhole)
	}

	connected := FIBEntryFromRouteRecord(FIBEntry{
		AFI:       "ipv4",
		Prefix:    "192.0.2.1/32",
		Protocol:  "connected",
		Installed: true,
	})
	if connected.Action != ActionReceive {
		t.Fatalf("connected no-next-hop action = %q, want %q", connected.Action, ActionReceive)
	}

	forward := FIBEntryFromRouteRecord(FIBEntry{
		AFI:      "ipv4",
		Prefix:   "10.0.0.0/24",
		Protocol: "bgp",
		NextHops: []NextHop{{Address: "192.0.2.1", Interface: "eth1", Weight: 1}},
	})
	if forward.Action != ActionForward || len(forward.NextHops) != 1 || forward.NextHops[0].Interface != "eth1" {
		t.Fatalf("forward conversion = %#v", forward)
	}
}
