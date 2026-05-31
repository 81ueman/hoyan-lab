package rib

import (
	"strconv"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

func parseASPath(raw string) []uint32 {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, ",", " "))
	if raw == "" || raw == "-" {
		return nil
	}
	var out []uint32
	for _, f := range strings.Fields(raw) {
		f = strings.Trim(f, "{}[]()")
		asn, err := strconv.ParseUint(f, 10, 32)
		if err == nil {
			out = append(out, uint32(asn))
		}
	}
	return out
}

func normalizeOrigin(origin string) model.BGPOriginCode {
	switch strings.ToLower(strings.TrimSpace(origin)) {
	case "", "i", "igp":
		return model.BGPOriginIGP
	case "e", "egp":
		return model.BGPOriginEGP
	case "?", "incomplete":
		return model.BGPOriginIncomplete
	default:
		return model.NormalizeBGPOriginCode(model.BGPOriginCode(origin))
	}
}

func defaultLocalPref(v int) int {
	if v == 0 {
		return 100
	}
	return v
}

func sortRoutes(routes []RIBRoute) {
	observation.SortRoutes(routes)
}

func sortPaths(paths []RIBPath, opts CompareOptions) {
	observation.SortPaths(paths, opts)
}

func normalizeRoute(route RIBRoute) RIBRoute {
	return observation.NormalizeRIBRouteRecord(route)
}
