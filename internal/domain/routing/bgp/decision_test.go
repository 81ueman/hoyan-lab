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

func bgpRoute(attrs route.BGPAttributes) route.RIBEntry {
	if attrs.ASPath == nil {
		attrs.ASPath = []uint32{65100}
	}
	return route.RIBEntry{
		Attrs: attrs,
	}
}

func TestLessWeight(t *testing.T) {
	// Weight is the highest priority criterion in BGP decision process.
	// Higher weight wins, compared before LocalPref.
	node := model.Node{Name: "r1"}
	opts := DecisionOptions{}

	t.Run("higher weight wins when local-pref equal", func(t *testing.T) {
		a := makeRoute(route.BGPAttributes{Weight: 200, LocalPref: 100})
		b := makeRoute(route.BGPAttributes{Weight: 100, LocalPref: 100})
		if !less(node, a, b, opts, false) {
			t.Errorf("expected a (Weight=200) to be preferred over b (Weight=100)")
		}
		if less(node, b, a, opts, false) {
			t.Errorf("expected b (Weight=100) to NOT be preferred over a (Weight=200)")
		}
	})

	t.Run("weight compared before local-pref", func(t *testing.T) {
		a := makeRoute(route.BGPAttributes{Weight: 200, LocalPref: 50})
		b := makeRoute(route.BGPAttributes{Weight: 100, LocalPref: 200})
		if !less(node, a, b, opts, false) {
			t.Errorf("expected a (Weight=200, LocalPref=50) to be preferred over b (Weight=100, LocalPref=200) because weight comes first")
		}
	})

	t.Run("equal weight falls through to local-pref", func(t *testing.T) {
		a := makeRoute(route.BGPAttributes{Weight: 100, LocalPref: 200})
		b := makeRoute(route.BGPAttributes{Weight: 100, LocalPref: 100})
		if !less(node, a, b, opts, false) {
			t.Errorf("expected a with higher local-pref to be preferred when weights equal")
		}
	})
}

func TestLessClusterList(t *testing.T) {
	// Cluster-List: shorter list is preferred.
	// Compared after eBGP/iBGP, before path tie-break.
	node := model.Node{Name: "r1"}
	opts := DecisionOptions{}

	t.Run("shorter cluster-list wins", func(t *testing.T) {
		a := bgpRoute(route.BGPAttributes{ASPath: []uint32{65100}, ClusterList: []string{"1.1.1.1"}})
		b := bgpRoute(route.BGPAttributes{ASPath: []uint32{65100}, ClusterList: []string{"1.1.1.1", "2.2.2.2"}})
		if !less(node, a, b, opts, false) {
			t.Errorf("expected a with shorter cluster-list to be preferred")
		}
		if less(node, b, a, opts, false) {
			t.Errorf("expected b with longer cluster-list to NOT be preferred")
		}
	})

	t.Run("equal cluster-list length falls through", func(t *testing.T) {
		a := bgpRoute(route.BGPAttributes{ASPath: []uint32{65100}, ClusterList: []string{"1.1.1.1"}})
		b := bgpRoute(route.BGPAttributes{ASPath: []uint32{65100}, ClusterList: []string{"2.2.2.2"}})
		// equal length, should fall through; verify by using higher local-pref
		a.Attrs.LocalPref = 200
		b.Attrs.LocalPref = 100
		if !less(node, a, b, opts, false) {
			t.Errorf("expected a with higher local-pref to win when cluster-list lengths equal")
		}
	})
}

func TestLessOriginatorID(t *testing.T) {
	// Originator-ID: lower value preferred when both are set.
	node := model.Node{Name: "r1"}

	t.Run("lower originator-id wins with PreferLowerRouterID", func(t *testing.T) {
		opts := DecisionOptions{PreferLowerRouterID: true}
		a := bgpRoute(route.BGPAttributes{ASPath: []uint32{65100}, OriginatorID: "1.1.1.1"})
		b := bgpRoute(route.BGPAttributes{ASPath: []uint32{65100}, OriginatorID: "2.2.2.2"})
		if !less(node, a, b, opts, false) {
			t.Errorf("expected a (OriginatorID=1.1.1.1) to be preferred over b (OriginatorID=2.2.2.2)")
		}
		if less(node, b, a, opts, false) {
			t.Errorf("expected b (OriginatorID=2.2.2.2) to NOT be preferred over a (OriginatorID=1.1.1.1)")
		}
	})

	t.Run("higher originator-id wins with PreferLowerRouterID=false", func(t *testing.T) {
		opts := DecisionOptions{PreferLowerRouterID: false}
		a := bgpRoute(route.BGPAttributes{ASPath: []uint32{65100}, OriginatorID: "2.2.2.2"})
		b := bgpRoute(route.BGPAttributes{ASPath: []uint32{65100}, OriginatorID: "1.1.1.1"})
		if !less(node, a, b, opts, false) {
			t.Errorf("expected a (OriginatorID=2.2.2.2) to be preferred over b (OriginatorID=1.1.1.1) when PreferLowerRouterID=false")
		}
	})

	t.Run("empty originator-id skips comparison", func(t *testing.T) {
		opts := DecisionOptions{PreferLowerRouterID: true}
		a := bgpRoute(route.BGPAttributes{ASPath: []uint32{65100}, OriginatorID: "1.1.1.1", LocalPref: 200})
		b := bgpRoute(route.BGPAttributes{ASPath: []uint32{65100}, OriginatorID: "", LocalPref: 100})
		// When one OriginatorID is empty, comparison should fall through to local-pref
		if !less(node, a, b, opts, false) {
			t.Errorf("expected a with higher local-pref to win when originator-id comparison skipped")
		}
		if less(node, b, a, opts, false) {
			t.Errorf("expected b with lower local-pref to NOT be preferred when originator-id comparison skipped")
		}
	})
}

func TestLessIGPCost(t *testing.T) {
	// IGP cost to next-hop: lower wins. Compared after eBGP/iBGP.
	node := model.Node{Name: "r1"}
	opts := DecisionOptions{}

	t.Run("lower igp cost wins", func(t *testing.T) {
		a := bgpRoute(route.BGPAttributes{ASPath: []uint32{65100}})
		a.IGPCost = 10
		b := bgpRoute(route.BGPAttributes{ASPath: []uint32{65100}})
		b.IGPCost = 20
		if !less(node, a, b, opts, false) {
			t.Errorf("expected a with IGPCost=10 to be preferred over b with IGPCost=20")
		}
		if less(node, b, a, opts, false) {
			t.Errorf("expected b with IGPCost=20 to NOT be preferred over a with IGPCost=10")
		}
	})

	t.Run("equal igp cost falls through", func(t *testing.T) {
		a := bgpRoute(route.BGPAttributes{ASPath: []uint32{65100}, LocalPref: 200})
		a.IGPCost = 10
		b := bgpRoute(route.BGPAttributes{ASPath: []uint32{65100}, LocalPref: 100})
		b.IGPCost = 10
		if !less(node, a, b, opts, false) {
			t.Errorf("expected a with higher LocalPref to win when IGPCost equal")
		}
	})

	t.Run("igp cost compared before cluster-list", func(t *testing.T) {
		a := bgpRoute(route.BGPAttributes{ASPath: []uint32{65100}, ClusterList: []string{"1.1.1.1"}})
		a.IGPCost = 5
		b := bgpRoute(route.BGPAttributes{ASPath: []uint32{65100}, ClusterList: []string{}})
		b.IGPCost = 10
		// a has IGP cost 5 and cluster-list 1 element, b has IGP cost 10 and empty cluster-list
		// IGP cost comparison should happen BEFORE cluster-list
		if !less(node, a, b, opts, false) {
			t.Errorf("expected a with lower IGPCost (5) to be preferred over b with higher IGPCost (10), even though b has shorter cluster-list")
		}
	})
}

func TestDeterministicMED(t *testing.T) {
	// DeterministicMED ensures MED is only compared within the same neighboring AS,
	// even when AlwaysCompareMED is enabled.
	// When all higher-priority criteria are equal, MED becomes the tie-breaker.
	// DeterministicMED prevents MED from being used across different neighbor ASes.
	node := model.Node{Name: "r1"}

	// routeEqualExceptMED creates two routes that are identical on all criteria
	// before MED in the decision process, differing only in MED and ASPath.
	routesEqualBeforeMED := func(asPathA, asPathB []uint32, medA, medB int) (route.RIBEntry, route.RIBEntry) {
		a := bgpRoute(route.BGPAttributes{
			ASPath:    asPathA,
			MED:       medA,
			LocalPref: 100,
		})
		b := bgpRoute(route.BGPAttributes{
			ASPath:    asPathB,
			MED:       medB,
			LocalPref: 100,
		})
		return a, b
	}

	t.Run("DeterministicMED prevents cross-AS MED comparison with AlwaysCompareMED", func(t *testing.T) {
		// Both routes have same local-pref, weight, as-path length, origin, etc.
		// Different neighboring AS (65100 vs 65200).
		// With DeterministicMED + AlwaysCompareMED, MED should NOT be compared
		// across different ASes, so lower MED does not determine the winner.
		opts := DecisionOptions{DeterministicMED: true, AlwaysCompareMED: true}
		a, b := routesEqualBeforeMED([]uint32{65100}, []uint32{65200}, 10, 50)
		// Different AS → MED skipped → falls through to eBGP/iBGP (both eBGP)
		// → IGP cost (both 0) → Cluster-List (both empty) → OriginatorID (both empty)
		// → CompareRouterID (false) → path tie-break (both same PathNodes)
		// With no path nodes set, both will compare as equal strings → tie
		// The sort will consider them equal (less returns false both ways)
		aLessB := less(node, a, b, opts, false)
		bLessA := less(node, b, a, opts, false)
		if aLessB || bLessA {
			t.Errorf("expected MED not to be compared across AS with DeterministicMED, got aLessB=%v bLessA=%v", aLessB, bLessA)
		}
	})

	t.Run("DeterministicMED allows MED comparison within same AS", func(t *testing.T) {
		// Both routes have same AS. MED should be compared.
		opts := DecisionOptions{DeterministicMED: true, AlwaysCompareMED: true}
		a, b := routesEqualBeforeMED([]uint32{65100}, []uint32{65100}, 10, 50)
		if !less(node, a, b, opts, false) {
			t.Errorf("expected a with lower MED (10) to be preferred within same AS")
		}
		if less(node, b, a, opts, false) {
			t.Errorf("expected b with higher MED (50) to NOT be preferred within same AS")
		}
	})

	t.Run("DeterministicMED without AlwaysCompareMED still works", func(t *testing.T) {
		opts := DecisionOptions{DeterministicMED: true, AlwaysCompareMED: false}
		a, b := routesEqualBeforeMED([]uint32{65100}, []uint32{65200}, 10, 50)
		// Different AS, MED should be skipped
		aLessB := less(node, a, b, opts, false)
		bLessA := less(node, b, a, opts, false)
		if aLessB || bLessA {
			t.Errorf("expected MED not to be compared across AS, got aLessB=%v bLessA=%v", aLessB, bLessA)
		}
	})

	t.Run("Without DeterministicMED, AlwaysCompareMED allows cross-AS MED", func(t *testing.T) {
		opts := DecisionOptions{DeterministicMED: false, AlwaysCompareMED: true}
		a, b := routesEqualBeforeMED([]uint32{65100}, []uint32{65200}, 10, 50)
		// Different AS but AlwaysCompareMED is true, MED should be compared
		if !less(node, a, b, opts, false) {
			t.Errorf("expected a with lower MED (10) to win with AlwaysCompareMED even across AS")
		}
	})

	t.Run("Without DeterministicMED, default behavior skips cross-AS MED", func(t *testing.T) {
		opts := DecisionOptions{DeterministicMED: false, AlwaysCompareMED: false}
		a, b := routesEqualBeforeMED([]uint32{65100}, []uint32{65200}, 10, 50)
		// Default: no AlwaysCompareMED, different AS → MED skipped
		aLessB := less(node, a, b, opts, false)
		bLessA := less(node, b, a, opts, false)
		if aLessB || bLessA {
			t.Errorf("expected MED not to be compared across AS by default, got aLessB=%v bLessA=%v", aLessB, bLessA)
		}
	})
}

func TestLessRouterID(t *testing.T) {
	// Router-ID: compared by FromNode name when CompareRouterID is enabled.
	optsOn := DecisionOptions{CompareRouterID: true, PreferLowerRouterID: true}
	optsOff := DecisionOptions{CompareRouterID: false, PreferLowerRouterID: true}
	node := model.Node{Name: "r1"}

	t.Run("lower from-node wins when CompareRouterID enabled", func(t *testing.T) {
		a := bgpRoute(route.BGPAttributes{ASPath: []uint32{65100}})
		a.Provenance.FromNode = "r2"
		b := bgpRoute(route.BGPAttributes{ASPath: []uint32{65100}})
		b.Provenance.FromNode = "r3"
		if !less(node, a, b, optsOn, false) {
			t.Errorf("expected a (FromNode=r2) to be preferred over b (FromNode=r3) when CompareRouterID enabled")
		}
		if less(node, b, a, optsOn, false) {
			t.Errorf("expected b (FromNode=r3) to NOT be preferred over a (FromNode=r2) when CompareRouterID enabled")
		}
	})

	t.Run("router-id not compared when CompareRouterID disabled", func(t *testing.T) {
		a := bgpRoute(route.BGPAttributes{ASPath: []uint32{65100}})
		a.Provenance.FromNode = "r2"
		a.Attrs.LocalPref = 200
		b := bgpRoute(route.BGPAttributes{ASPath: []uint32{65100}})
		b.Provenance.FromNode = "r3"
		b.Attrs.LocalPref = 100
		// With CompareRouterID=false, result should be determined by local-pref, not FromNode
		if !less(node, a, b, optsOff, false) {
			t.Errorf("expected a with higher local-pref to win when CompareRouterID=false")
		}
		if less(node, b, a, optsOff, false) {
			t.Errorf("expected b with lower local-pref to NOT be preferred when CompareRouterID=false")
		}
	})

	t.Run("same from-node skips router-id comparison", func(t *testing.T) {
		a := bgpRoute(route.BGPAttributes{ASPath: []uint32{65100}})
		a.Provenance.FromNode = "r2"
		a.Attrs.LocalPref = 200
		b := bgpRoute(route.BGPAttributes{ASPath: []uint32{65100}})
		b.Provenance.FromNode = "r2"
		b.Attrs.LocalPref = 100
		// Same FromNode, should fall through to local-pref
		if !less(node, a, b, optsOn, false) {
			t.Errorf("expected a with higher local-pref to win when same from-node")
		}
		if less(node, b, a, optsOn, false) {
			t.Errorf("expected b with lower local-pref to NOT be preferred when same from-node")
		}
	})
}
