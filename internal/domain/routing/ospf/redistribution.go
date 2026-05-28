package ospf

import (
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/routing/route"
)

func RedistributedExternalRoute(node string, redist model.OSPFRedistribution, in route.RIBEntry) route.RIBEntry {
	out := in.Normalize()
	source := out.RouteSource
	source.Node = node
	source.Kind = model.RouteSourceOSPF
	source.AdminDistance = 110
	source.MetricType = ExternalMetricType(redist.MetricType)
	source.OSPFRouteType = ExternalRouteType(source.MetricType)
	source.Metric = ExternalMetric(redist, out)
	out.SourceKind = model.RouteSourceOSPF
	out.RouteSource = source
	return out.Normalize()
}
