package observation

import "github.com/81ueman/hoyan-lab/internal/domain/model"

func NormalizeFIBEntries(routes []FIBEntry) ([]FIBEntry, []DuplicateRouteConflict) {
	return normalizeRoutesForSide("", routes, fibRouteKey)
}

func normalizeRoutesForSide(side string, routes []FIBEntry, keyFunc func(FIBEntry) string) ([]FIBEntry, []DuplicateRouteConflict) {
	entries := map[string]routeIndexEntry{}
	for _, route := range routes {
		route = normalizeFIBEntryForCompare(route)
		route.NextHops = dedupeFIBNextHops(route.NextHops)
		key := keyFunc(route)
		entry, ok := entries[key]
		if !ok {
			entries[key] = routeIndexEntry{route: route, routes: []FIBEntry{route}}
			continue
		}
		merged, reason, ok := mergeDuplicateRoute(entry.route, route)
		entry.routes = append(entry.routes, route)
		if !ok {
			entry.conflicted = true
			if entry.reason == "" {
				entry.reason = reason
			}
		} else {
			entry.route = merged
		}
		entries[key] = entry
	}

	out := make([]FIBEntry, 0, len(entries))
	var conflicts []DuplicateRouteConflict
	for key, entry := range entries {
		if entry.conflicted {
			conflicts = append(conflicts, DuplicateRouteConflict{
				RouteKey: key,
				Side:     side,
				Reason:   entry.reason,
				Routes:   entry.routes,
			})
			continue
		}
		out = append(out, entry.route)
	}
	sortFIBEntriesForCompare(out)
	sortDuplicateRouteConflicts(conflicts)
	return out, conflicts
}

func normalizeFIBEntryForCompare(route FIBEntry) FIBEntry {
	route.Source.Protocol = model.NormalizeRouteSourceKind(route.Source.Protocol)
	if route.AFI == "" {
		route.AFI = model.AFIIPv4
	} else {
		route.AFI = model.NormalizeAFI(route.AFI)
	}
	if route.Action == "" {
		if route.Source.Protocol == model.RouteSourceBlackhole {
			route.Action = ActionDrop
		} else if route.Source.Protocol == model.RouteSourceConnected && len(route.NextHops) == 0 {
			route.Action = ActionReceive
		} else {
			route.Action = ActionForward
		}
	}
	return route
}

type routeIndexEntry struct {
	route      FIBEntry
	routes     []FIBEntry
	conflicted bool
	reason     string
}

func mergeDuplicateRoute(a, b FIBEntry) (FIBEntry, string, bool) {
	conflict := func(field string) (FIBEntry, string, bool) {
		return FIBEntry{}, field + " mismatch", false
	}
	if a.AFI != b.AFI {
		return conflict("afi")
	}
	if a.Prefix != b.Prefix {
		return conflict("prefix")
	}
	if model.NormalizeRouteSourceKind(a.Source.Protocol) != model.NormalizeRouteSourceKind(b.Source.Protocol) {
		return conflict("protocol")
	}
	if a.Preference != 0 && b.Preference != 0 && a.Preference != b.Preference {
		return conflict("preference")
	}
	if a.Metric != 0 && b.Metric != 0 && a.Metric != b.Metric {
		return conflict("metric")
	}

	merged := a
	if merged.Preference == 0 {
		merged.Preference = b.Preference
	}
	if merged.Metric == 0 {
		merged.Metric = b.Metric
	}
	if merged.SAFI == "" {
		merged.SAFI = b.SAFI
	}
	if merged.TableID == "" {
		merged.TableID = b.TableID
	}
	if merged.TableName == "" {
		merged.TableName = b.TableName
	}
	if merged.ProtocolInstance == "" {
		merged.ProtocolInstance = b.ProtocolInstance
	}
	if merged.Age == "" {
		merged.Age = b.Age
	}
	if merged.AgeSeconds == 0 {
		merged.AgeSeconds = b.AgeSeconds
	}
	if merged.Tag == 0 {
		merged.Tag = b.Tag
	}
	if merged.InstalledReason == "" {
		merged.InstalledReason = b.InstalledReason
	}
	if merged.Raw == nil {
		merged.Raw = b.Raw
	}
	merged.NextHops = unionNextHops(a.NextHops, b.NextHops)
	return merged, "", true
}

func unionNextHops(a, b []NextHop) []NextHop {
	out := make([]NextHop, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b...)
	return dedupeFIBNextHops(out)
}
