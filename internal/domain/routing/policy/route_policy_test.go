package policy

import (
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

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

func TestPrefixListPermitsPrefixUsesNLRILengthSemantics(t *testing.T) {
	rule := model.PrefixListRule{Seq: 10, Action: "permit", Prefix: "10.0.0.0/8", Ge: 16, Le: 24}
	node := model.Node{PrefixLists: []model.PrefixList{{Name: "PL", Rules: []model.PrefixListRule{rule}}}}
	if !PrefixListPermitsPrefix(node, "PL", model.MustPrefix("10.4.0.0/16").NetIP()) {
		t.Fatalf("prefix-list range should match NLRI inside ge/le bounds")
	}
	if PrefixListPermitsPrefix(node, "PL", model.MustPrefix("10.4.1.10/32").NetIP()) {
		t.Fatalf("prefix-list range should reject NLRI longer than le")
	}
	if PrefixListPermitsPrefix(node, "PL", model.MustPrefix("10.0.0.0/8").NetIP()) {
		t.Fatalf("prefix-list range should reject NLRI shorter than ge")
	}
}
