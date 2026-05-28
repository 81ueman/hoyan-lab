package controlplane

const (
	RouteTypeIntraArea = "intra-area"
	RouteTypeInterArea = "inter-area"
	RouteTypeExternal1 = "external-type-1"
	RouteTypeExternal2 = "external-type-2"
	BackboneArea       = "0"
)

func RouteTypeRank(routeType string) int {
	switch routeType {
	case RouteTypeIntraArea, "":
		return 0
	case RouteTypeInterArea:
		return 1
	case RouteTypeExternal1:
		return 2
	case RouteTypeExternal2:
		return 3
	default:
		return 4
	}
}
