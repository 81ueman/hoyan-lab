package bgp

type OriginCode string

const (
	OriginIGP        OriginCode = "igp"
	OriginEGP        OriginCode = "egp"
	OriginIncomplete OriginCode = "incomplete"
)

func OriginCodeRank(origin string) int {
	switch OriginCode(origin) {
	case OriginIGP:
		return 0
	case OriginEGP:
		return 1
	case OriginIncomplete:
		return 2
	default:
		return 3
	}
}

func NeighboringAS(path []uint32) uint32 {
	if len(path) == 0 {
		return 0
	}
	return path[0]
}
