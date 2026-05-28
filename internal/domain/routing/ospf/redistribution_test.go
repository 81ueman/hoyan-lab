package ospf

import (
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/routing/route"
)

func TestRedistributedExternalRouteRewritesOSPFAttributes(t *testing.T) {
	in := route.RIBEntry{
		NLRI:       route.NLRI{Prefix: model.MustPrefix("203.0.113.0/24")},
		Attrs:      route.BGPAttributes{MED: 7},
		SourceKind: model.RouteSourceBGP,
		RouteSource: model.ConfiguredRoute{
			Node:            "r0",
			NetworkInstance: model.NetworkInstanceDefault,
			Prefix:          model.MustPrefix("203.0.113.0/24"),
			Kind:            model.RouteSourceBGP,
			Metric:          99,
		},
	}

	got := RedistributedExternalRoute("r1", model.OSPFRedistribution{Kind: model.RouteSourceBGP, MetricType: 1}, in)

	if got.SourceKind != model.RouteSourceOSPF {
		t.Fatalf("source kind = %q, want %q", got.SourceKind, model.RouteSourceOSPF)
	}
	if got.RouteSource.Node != "r1" || got.RouteSource.Kind != model.RouteSourceOSPF || got.RouteSource.AdminDistance != 110 {
		t.Fatalf("route source = %#v, want OSPF source from r1 with AD 110", got.RouteSource)
	}
	if got.RouteSource.MetricType != 1 || got.RouteSource.OSPFRouteType != RouteTypeExternal1 || got.RouteSource.Metric != 7 {
		t.Fatalf("OSPF attrs = metric_type %d route_type %q metric %d, want E1 metric 7", got.RouteSource.MetricType, got.RouteSource.OSPFRouteType, got.RouteSource.Metric)
	}
}

func TestExternalMetricPrecedence(t *testing.T) {
	entry := route.RIBEntry{
		NLRI:        route.NLRI{Prefix: model.MustPrefix("198.51.100.0/24")},
		Attrs:       route.BGPAttributes{MED: 30},
		RouteSource: model.ConfiguredRoute{Metric: 40},
	}

	if got := ExternalMetric(model.OSPFRedistribution{Metric: 20}, entry); got != 20 {
		t.Fatalf("explicit metric = %d, want 20", got)
	}
	if got := ExternalMetric(model.OSPFRedistribution{}, entry); got != 30 {
		t.Fatalf("MED metric = %d, want 30", got)
	}
	entry.Attrs.MED = 0
	if got := ExternalMetric(model.OSPFRedistribution{}, entry); got != 40 {
		t.Fatalf("source metric = %d, want 40", got)
	}
	entry.RouteSource.Metric = 0
	if got := ExternalMetric(model.OSPFRedistribution{}, entry); got != 20 {
		t.Fatalf("default metric = %d, want 20", got)
	}
}
