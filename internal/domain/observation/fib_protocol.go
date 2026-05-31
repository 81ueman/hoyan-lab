package observation

import "github.com/81ueman/hoyan-lab/internal/domain/model"

func canonicalProtocol(protocol string) string {
	return string(model.NormalizeRouteSourceKind(model.RouteSourceKind(protocol)))
}

func CanonicalProtocol(protocol string) string {
	return canonicalProtocol(protocol)
}
