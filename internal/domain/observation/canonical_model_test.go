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
