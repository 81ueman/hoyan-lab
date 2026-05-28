package bgp

import (
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func TestOriginCodeRank(t *testing.T) {
	if OriginCodeRank(string(OriginIGP)) >= OriginCodeRank(string(OriginIncomplete)) {
		t.Fatalf("IGP origin should rank before incomplete")
	}
}

func TestCommunityListPermitsExact(t *testing.T) {
	node := model.Node{CommunityLists: []model.CommunityList{{
		Name: "COMM",
		Rules: []model.StringListRule{
			{Action: "permit", Pattern: "65100:1"},
			{Action: "permit", Pattern: "65100:2"},
		},
	}}}
	if !CommunityListPermits(node, "COMM", []string{"65100:1", "65100:2"}, true) {
		t.Fatalf("CommunityListPermits() = false, want exact permit")
	}
	if CommunityListPermits(node, "COMM", []string{"65100:1"}, true) {
		t.Fatalf("CommunityListPermits() = true, want exact mismatch")
	}
}
