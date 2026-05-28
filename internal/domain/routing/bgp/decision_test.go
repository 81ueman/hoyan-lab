package bgp

import "testing"

func TestOriginCodeRank(t *testing.T) {
	if OriginCodeRank(string(OriginIGP)) >= OriginCodeRank(string(OriginIncomplete)) {
		t.Fatalf("IGP origin should rank before incomplete")
	}
}
