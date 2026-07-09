package intent

import (
	"fmt"
	"net/netip"
	"regexp"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	"github.com/81ueman/hoyan-lab/internal/engine/controlplane"
)

// ---------------------------------------------------------------------------
// Row matching and filtering
// ---------------------------------------------------------------------------

// routeRow represents a matching RIB route with its parent RIB metadata.
type routeRow struct {
	rib   observation.RIB
	route observation.RIBRoute
}

// matchingRows returns all RIB routes from the snapshot that satisfy all
// given where predicates (AND semantics). Pass nil for no filter.
// Returns an error if any where predicate contains unrecognized keys.
func matchingRows(snapshot SnapshotContext, filters ...map[string]any) ([]routeRow, error) {
	var out []routeRow
	for _, rib := range RIBs(snapshot.Network) {
		for _, route := range rib.Routes {
			ok, err := matchAllWhere(route, rib, filters...)
			if err != nil {
				return nil, err
			}
			if ok {
				out = append(out, routeRow{rib: rib, route: route})
			}
		}
	}
	return out, nil
}

// matchAllWhere checks that a route satisfies all given where predicates (AND).
// Returns an error if any where predicate contains unrecognized keys.
func matchAllWhere(route observation.RIBRoute, rib observation.RIB, filters ...map[string]any) (bool, error) {
	for _, f := range filters {
		if len(f) > 0 {
			ok, err := matchWhere(route, rib, f)
			if err != nil {
				return false, err
			}
			if !ok {
				return false, nil
			}
		}
	}
	return true, nil
}

// validWhereKeys contains all recognized keys for RCL where predicates.
var validWhereKeys = map[string]bool{
	"prefix": true, "device": true, "node": true, "vrf": true,
	"protocol": true, "not": true, "and": true, "or": true,
	"device_in": true, "selected": true,
	"communities": true, "as_path": true, "weight": true, "connected_class": true,
	"contains": true, "matches": true, "imply": true, "prefix_within": true,
	"route_type": true, "area": true, "cost": true,
	"origin": true, "med": true, "large_communities": true,
	"peer": true, "peer_as": true,
	"as_path_len": true, "aspath_len": true,
	"nexthop": true,
}

// matchWhere checks if a RIB route matches a simple where predicate map.
// Supported keys: prefix, device, node, vrf, protocol, not, and, or.
// Multiple keys are ANDed together. The "not" key contains a nested predicate
// that is negated. Returns an error for unrecognized keys.
func matchWhere(route observation.RIBRoute, rib observation.RIB, where map[string]any) (bool, error) {
	if len(where) == 0 {
		return true, nil
	}

	for key, raw := range where {
		lkey := strings.ToLower(key)
		if !validWhereKeys[lkey] {
			return false, fmt.Errorf("unrecognized where key %q", key)
		}

		switch lkey {
		case "prefix":
			val, ok := raw.(string)
			if !ok {
				continue
			}
			if !matchesPrefix(route.Common.Prefix, val) {
				return false, nil
			}

		case "device", "node":
			val, ok := raw.(string)
			if !ok {
				continue
			}
			if string(rib.Node) != val {
				return false, nil
			}

		case "device_in":
			devices, ok := raw.([]any)
			if !ok {
				continue
			}
			found := false
			for _, d := range devices {
				if s, ok := d.(string); ok && s == string(rib.Node) {
					found = true
					break
				}
			}
			if !found {
				return false, nil
			}

		case "selected":
			want, ok := raw.(bool)
			if !ok {
				continue
			}
			if route.Common.Best != want {
				return false, nil
			}

		case "vrf":
			val, ok := raw.(string)
			if !ok {
				continue
			}
			if string(rib.VRF) != val {
				return false, nil
			}

		case "protocol":
			val, ok := raw.(string)
			if !ok {
				continue
			}
			if string(route.Common.Protocol) != val {
				return false, nil
			}

		case "not":
			inner, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			innerOk, err := matchWhere(route, rib, inner)
			if err != nil {
				return false, err
			}
			if innerOk {
				return false, nil // negate
			}

		case "and":
			conds, ok := raw.([]any)
			if !ok {
				continue
			}
			for _, c := range conds {
				inner, ok := c.(map[string]any)
				if !ok {
					return false, nil
				}
				innerOk, err := matchWhere(route, rib, inner)
				if err != nil {
					return false, err
				}
				if !innerOk {
					return false, nil
				}
			}

		case "or":
			conds, ok := raw.([]any)
			if !ok {
				continue
			}
			anyMatch := false
			for _, c := range conds {
				inner, ok := c.(map[string]any)
				if !ok {
					continue
				}
				innerOk, err := matchWhere(route, rib, inner)
				if err != nil {
					return false, err
				}
				if innerOk {
					anyMatch = true
					break
				}
			}
			if !anyMatch {
				return false, nil
			}

		case "communities":
			if opMap, ok := raw.(map[string]any); ok {
				if cv, ok := opMap["contains"]; ok {
					if !containsCheck(routeCommunities(route), cv) {
						return false, nil
					}
					continue
				}
			}
			continue

		case "large_communities":
			if opMap, ok := raw.(map[string]any); ok {
				if cv, ok := opMap["contains"]; ok {
					if !containsCheck(routeLargeCommunities(route), cv) {
						return false, nil
					}
					continue
				}
			}
			continue

		case "as_path":
			if opMap, ok := raw.(map[string]any); ok {
				if mv, ok := opMap["matches"]; ok {
					if !matchesCheck(routeASPath(route), scalar(mv)) {
						return false, nil
					}
					continue
				}
			}
			if routeASPath(route) != scalar(raw) {
				return false, nil
			}

		case "as_path_len", "aspath_len":
			want := toInt(raw)
			if routeASPathLen(route) != want {
				return false, nil
			}

		case "weight":
			if !valuesEqual(routeWeight(route), raw) {
				return false, nil
			}

		case "connected_class":
			val, ok := raw.(string)
			if !ok {
				continue
			}
			if string(routeConnectedClass(route)) != val {
				return false, nil
			}

		case "imply":
			clauses, ok := raw.([]any)
			if !ok || len(clauses) != 2 {
				continue
			}
			antecedent, aOK := clauses[0].(map[string]any)
			consequent, cOK := clauses[1].(map[string]any)
			if !aOK || !cOK {
				continue
			}
			aOk, err := matchWhere(route, rib, antecedent)
			if err != nil {
				return false, err
			}
			if !aOk {
				return true, nil
			}
			return matchWhere(route, rib, consequent)

		case "prefix_within":
			val, ok := raw.(string)
			if !ok {
				continue
			}
			if !prefixWithin(route.Common.Prefix, val) {
				return false, nil
			}

		case "peer":
			if !valuesEqual(routePeer(route), raw) {
				return false, nil
			}

		case "peer_as":
			if !valuesEqual(routePeerAS(route), raw) {
				return false, nil
			}

		case "nexthop", "next_hop":
			actual := extractNextHops(route)
			if val, ok := raw.(string); ok {
				if actual == "" || !strings.Contains(actual, val) {
					return false, nil
				}
			} else if opMap, ok := raw.(map[string]any); ok {
				if cv, ok := opMap["contains"]; ok {
					if !containsCheck(actual, cv) {
						return false, nil
					}
				} else if mv, ok := opMap["matches"]; ok {
					if !matchesCheck(actual, scalar(mv)) {
						return false, nil
					}
				}
			}
			continue

		default:
			if opMap, ok := raw.(map[string]any); ok {
				if cv, ok := opMap["contains"]; ok {
					actual := routeFieldValue(route, key)
					if !containsCheck(actual, cv) {
						return false, nil
					}
					continue
				}
				if mv, ok := opMap["matches"]; ok {
					actual := routeFieldValue(route, key)
					if !matchesCheck(actual, scalar(mv)) {
						return false, nil
					}
					continue
				}
			}
			actual := routeFieldValue(route, key)
			if !valuesEqual(actual, raw) {
				return false, nil
			}
		}
	}
	return true, nil
}

// matchesPrefix checks if the route prefix is within or equal to the given
// CIDR prefix string.
func matchesPrefix(routePrefix, wherePrefix string) bool {
	if routePrefix == "" || wherePrefix == "" {
		return routePrefix == wherePrefix
	}
	rp, err := model.ParsePrefix(routePrefix)
	if err != nil {
		return routePrefix == wherePrefix
	}
	wp, err := model.ParsePrefix(wherePrefix)
	if err != nil {
		return routePrefix == wherePrefix
	}
	// Check if routePrefix is within wherePrefix (subnet containment)
	if wp.Equal(rp) {
		return true
	}
	// Check if the route prefix address is within the where prefix range
	return wp.Contains(rp.Addr())
}

// ---------------------------------------------------------------------------
// Helper functions for where predicates
// ---------------------------------------------------------------------------

// scalar converts any value to its string representation.
func scalar(v any) string {
	switch s := v.(type) {
	case string:
		return s
	default:
		return fmt.Sprintf("%v", v)
	}
}

// valuesEqual compares two values for equality using string representation.
func valuesEqual(a, b any) bool {
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

// routeLargeCommunities returns the BGP large communities for a route.
func routeLargeCommunities(route observation.RIBRoute) []string {
	if route.BGP != nil && len(route.BGP.Paths) > 0 {
		return route.BGP.Paths[0].LargeCommunities
	}
	return nil
}

// routeCommunities returns the BGP communities for a route.
func routeCommunities(route observation.RIBRoute) []string {
	if route.BGP != nil && len(route.BGP.Paths) > 0 {
		return route.BGP.Paths[0].Communities
	}
	return nil
}

// routeASPath returns the string representation of the AS_PATH for a route.
func routeASPath(route observation.RIBRoute) string {
	if route.BGP != nil && len(route.BGP.Paths) > 0 {
		return fmt.Sprintf("%v", route.BGP.Paths[0].ASPath)
	}
	return ""
}

// routeASPathLen returns the length of the AS_PATH for a route.
func routeASPathLen(route observation.RIBRoute) int {
	if route.BGP != nil && len(route.BGP.Paths) > 0 {
		return len(route.BGP.Paths[0].ASPath)
	}
	return 0
}

// routeWeight returns the BGP weight for a route.
func routeWeight(route observation.RIBRoute) int {
	if route.BGP != nil && len(route.BGP.Paths) > 0 {
		return route.BGP.Paths[0].Weight
	}
	return 0
}

// routePeer returns the BGP peer address for a route.
func routePeer(route observation.RIBRoute) string {
	if route.BGP != nil && len(route.BGP.Paths) > 0 {
		return route.BGP.Paths[0].Peer
	}
	return ""
}

// routePeerAS returns the BGP peer AS number for a route.
func routePeerAS(route observation.RIBRoute) int {
	if route.BGP != nil && len(route.BGP.Paths) > 0 {
		return int(route.BGP.Paths[0].PeerAS)
	}
	return 0
}

// routeConnectedClass returns a simplified connected route class string.
func routeConnectedClass(route observation.RIBRoute) string {
	if route.Connected != nil && route.Connected.Interface != "" {
		if controlplane.IsLoopbackInterface(route.Connected.Interface) {
			return "loopback"
		}
		return "link"
	}
	return ""
}

// containsCheck checks if an actual value contains a target value.
// For []string, it checks if any element matches. For strings, it checks substring.
func containsCheck(actual any, containsVal any) bool {
	want := scalar(containsVal)
	switch a := actual.(type) {
	case []string:
		for _, v := range a {
			if v == want {
				return true
			}
		}
		return false
	case string:
		return strings.Contains(a, want)
	default:
		return strings.Contains(scalar(a), want)
	}
}

// matchesCheck checks if an actual value matches a regex pattern.
func matchesCheck(actual any, pattern string) bool {
	re := regexp.MustCompile(pattern)
	switch a := actual.(type) {
	case []string:
		return re.MatchString(strings.Join(a, " "))
	default:
		return re.MatchString(scalar(actual))
	}
}

// prefixWithin checks if prefix is contained within parent CIDR.
func prefixWithin(prefix, parent string) bool {
	p, err := netip.ParsePrefix(prefix)
	if err != nil {
		return false
	}
	container, err := netip.ParsePrefix(parent)
	if err != nil {
		return false
	}
	return container.Contains(p.Addr()) && p.Bits() >= container.Bits()
}
