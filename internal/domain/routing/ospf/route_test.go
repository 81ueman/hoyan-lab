package ospf

import "testing"

func TestRouteTypeRank(t *testing.T) {
	if RouteTypeRank(RouteTypeIntraArea) >= RouteTypeRank(RouteTypeExternal2) {
		t.Fatalf("intra-area should rank before external type 2")
	}
}
