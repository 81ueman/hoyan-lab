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

func TestFIBEntryValidateRequiresCanonicalForwardingFields(t *testing.T) {
	entry := FIBEntry{
		AFI:      model.AFIIPv4,
		Prefix:   "10.0.0.0/24",
		Source:   RouteSource{Protocol: model.RouteSourceBGP},
		Action:   ActionForward,
		NextHops: []NextHop{{Address: "192.0.2.1", Interface: "eth1"}},
	}
	if err := entry.Validate(); err != nil {
		t.Fatalf("canonical forward entry failed validation: %v", err)
	}
	entry.NextHops = nil
	if err := entry.Validate(); err == nil {
		t.Fatalf("forward entry without next-hop passed validation")
	}
}
