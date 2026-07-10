package bgp

import (
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/routing/route"
)

func TestOriginCodeRank(t *testing.T) {
	if OriginCodeRank(model.BGPOriginIGP) >= OriginCodeRank(model.BGPOriginIncomplete) {
		t.Fatalf("IGP origin should rank before incomplete")
	}
}

func makeRoute(attrs route.BGPAttributes) route.RIBEntry {
	return route.RIBEntry{
		Attrs: attrs,
	}
}

func TestLessWeight(t *testing.T) {
	// Weight is the highest priority criterion in BGP decision process.
	// Higher weight wins, compared before LocalPref.
	node := model.Node{Name: "r1"}

	t.Run("higher weight wins when local-pref equal", func(t *testing.T) {
		a := makeRoute(route.BGPAttributes{Weight: 200, LocalPref: 100})
		b := makeRoute(route.BGPAttributes{Weight: 100, LocalPref: 100})
		if !less(node, a, b, func(_, _ route.RIBEntry) bool { return false }, false) {
			t.Errorf("expected a (Weight=200) to be preferred over b (Weight=100)")
		}
		if less(node, b, a, func(_, _ route.RIBEntry) bool { return false }, false) {
			t.Errorf("expected b (Weight=100) to NOT be preferred over a (Weight=200)")
		}
	})

	t.Run("weight compared before local-pref", func(t *testing.T) {
		// Higher weight with lower local-pref should still win over
		// lower weight with higher local-pref, because weight comes first.
		a := makeRoute(route.BGPAttributes{Weight: 200, LocalPref: 50})
		b := makeRoute(route.BGPAttributes{Weight: 100, LocalPref: 200})
		if !less(node, a, b, func(_, _ route.RIBEntry) bool { return false }, false) {
			t.Errorf("expected a (Weight=200, LocalPref=50) to be preferred over b (Weight=100, LocalPref=200) because weight comes first")
		}
	})

	t.Run("equal weight falls through to local-pref", func(t *testing.T) {
		a := makeRoute(route.BGPAttributes{Weight: 100, LocalPref: 200})
		b := makeRoute(route.BGPAttributes{Weight: 100, LocalPref: 100})
		if !less(node, a, b, func(_, _ route.RIBEntry) bool { return false }, false) {
			t.Errorf("expected a with higher local-pref to be preferred when weights equal")
		}
	})
}
