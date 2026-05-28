package ospf

import (
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/routing/route"
)

func ExternalMetric(redist model.OSPFRedistribution, in route.RIBEntry) int {
	in = in.Normalize()
	if redist.Metric > 0 {
		return redist.Metric
	}
	if in.Attrs.MED > 0 {
		return in.Attrs.MED
	}
	if in.RouteSource.Metric > 0 {
		return in.RouteSource.Metric
	}
	return 20
}
