package controlplane

func ExternalRouteType(metricType int) string {
	if ExternalMetricType(metricType) == 1 {
		return RouteTypeExternal1
	}
	return RouteTypeExternal2
}

func ExternalMetricType(metricType int) int {
	if metricType == 1 {
		return 1
	}
	return 2
}
