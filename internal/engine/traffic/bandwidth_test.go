package traffic

import (
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func TestLinkBandwidthExplicit(t *testing.T) {
	link := model.Link{
		Name:      "test-link",
		Bandwidth: 25_000_000_000, // 25 Gbps
		Role:      "core",
	}
	bw := linkBandwidth(link, &model.Topology{})
	if bw != 25_000_000_000 {
		t.Errorf("expected 25 Gbps, got %d", bw)
	}
}

func TestLinkBandwidthRoleDefault(t *testing.T) {
	tests := []struct {
		role string
		want uint64
	}{
		{"core", 40_000_000_000},
		{"edge", 10_000_000_000},
		{"border", 10_000_000_000},
		{"customer", 1_000_000_000},
		{"unknown", 0},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			link := model.Link{
				Name: "test-link",
				Role: tt.role,
			}
			bw := linkBandwidth(link, &model.Topology{})
			if bw != tt.want {
				t.Errorf("linkBandwidth() = %d, want %d", bw, tt.want)
			}
		})
	}
}

func TestBandwidthOverride(t *testing.T) {
	topo := &model.Topology{
		Links: []model.Link{
			{Name: "link1", Role: "core"},
			{Name: "link2", Role: "edge"},
		},
	}

	overrides := BandwidthOverride{
		"link1": 100_000_000_000, // 100 Gbps
	}

	ApplyBandwidthOverrides(topo, overrides)

	if topo.Links[0].Bandwidth != 100_000_000_000 {
		t.Errorf("link1: expected 100 Gbps, got %d", topo.Links[0].Bandwidth)
	}
	if topo.Links[1].Bandwidth != 0 {
		t.Errorf("link2: expected 0 (no override), got %d", topo.Links[1].Bandwidth)
	}

	// link with override should use explicit bandwidth
	bw := linkBandwidth(topo.Links[0], topo)
	if bw != 100_000_000_000 {
		t.Errorf("linkBandwidth(link1) = %d, want 100 Gbps", bw)
	}

	// link without override should use role default
	bw = linkBandwidth(topo.Links[1], topo)
	if bw != 10_000_000_000 {
		t.Errorf("linkBandwidth(link2) = %d, want 10 Gbps", bw)
	}
}
