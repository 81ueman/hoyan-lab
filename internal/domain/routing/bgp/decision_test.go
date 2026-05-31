package bgp

import (
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func TestOriginCodeRank(t *testing.T) {
	if OriginCodeRank(model.BGPOriginIGP) >= OriginCodeRank(model.BGPOriginIncomplete) {
		t.Fatalf("IGP origin should rank before incomplete")
	}
}
