package observation

import (
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func TestAFIFilteringFromCollectOptions(t *testing.T) {
	routes := []RIBRoute{
		{
			Common: RIBRouteCommon{
				AFI: model.AFIIPv4, Prefix: "10.0.0.0/24", Protocol: model.RouteSourceBGP,
				Eligible: true, Best: true,
			},
			BGP: &BGPRIBRoute{},
		},
		{
			Common: RIBRouteCommon{
				AFI: model.AFIIPv6, Prefix: "2001:db8::/32", Protocol: model.RouteSourceBGP,
				Eligible: true, Best: true,
			},
			BGP: &BGPRIBRoute{},
		},
	}

	// IPv4 filter
	filtered := FilterRIB(RIB{Node: "test", VRF: "default", Routes: routes}, CollectOptions{AFI: model.AFIIPv4})
	if len(filtered.Routes) != 1 || filtered.Routes[0].Common.AFI != model.AFIIPv4 {
		t.Fatalf("IPv4 filter: got %d routes, want 1 IPv4", len(filtered.Routes))
	}

	// IPv6 filter
	filtered = FilterRIB(RIB{Node: "test", VRF: "default", Routes: routes}, CollectOptions{AFI: model.AFIIPv6})
	if len(filtered.Routes) != 1 || filtered.Routes[0].Common.AFI != model.AFIIPv6 {
		t.Fatalf("IPv6 filter: got %d routes, want 1 IPv6", len(filtered.Routes))
	}

	// No filter returns both
	filtered = FilterRIB(RIB{Node: "test", VRF: "default", Routes: routes}, CollectOptions{})
	if len(filtered.Routes) != 2 {
		t.Fatalf("no filter: got %d routes, want 2", len(filtered.Routes))
	}
}
