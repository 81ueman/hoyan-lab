package bgp

import "github.com/81ueman/hoyan-lab/internal/domain/model"

func OriginCodeRank(origin model.BGPOriginCode) int {
	switch model.NormalizeBGPOriginCode(origin) {
	case model.BGPOriginIGP:
		return 0
	case model.BGPOriginEGP:
		return 1
	case model.BGPOriginIncomplete:
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
