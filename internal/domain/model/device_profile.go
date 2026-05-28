package model

import (
	"sort"
	"strings"
	"sync"
)

type DeviceProfile interface {
	Kind() DeviceKind
	InterfaceProfile() InterfaceProfile
	ACLProfile() ACLProfile
	FIBProfile() FIBProfile
	LiveProfile() LiveProfile
	ConfigProfile() ConfigProfile
}

type InterfaceProfile interface {
	InterfaceAliases(name string) []string
	CanonicalInterfaceName(name string) string
	EquivalentInterfaceName(a, b string) bool
	IsManagementInterface(interfaces []Interface, name string) bool
	IsLoopbackInterface(name string) bool
}

type ACLProfile interface {
	DefaultACLAction(fallback ACLDefaultAction) ACLDefaultAction
}

type FIBProfile interface {
	ExpectedFIBRouteVisible(source RouteSourceKind, class ConnectedRouteClass) bool
}

type LiveProfile interface {
	SupportsBGPRIBCollection() bool
	SupportsRouteTableCollection() bool
	SupportsFIBCollection() bool
	BGPRIBCollector() (LiveCollectorID, bool)
	RouteTableCollector() (LiveCollectorID, bool)
	FIBCollector() (LiveCollectorID, bool)
	ShouldCollectBGP(node Node) bool
	IncludeInBGPRIBCollection(node Node) bool
}

type ConfigProfile interface {
	SupportsConfigParse() bool
	SupportsOSPFConfig() bool
	SupportsRouteMapPolicy() bool
	OSPFInterfaceVRF(interfaces []Interface, name string) NetworkInstanceID
	RouteMapVendorName() string
}

type deviceProfile struct {
	kind   DeviceKind
	ifaces InterfaceProfile
	acl    ACLProfile
	fib    FIBProfile
	live   LiveProfile
	config ConfigProfile
}

func (p deviceProfile) Kind() DeviceKind {
	return p.kind
}

func (p deviceProfile) InterfaceProfile() InterfaceProfile {
	return p.ifaces
}

func (p deviceProfile) ACLProfile() ACLProfile {
	return p.acl
}

func (p deviceProfile) FIBProfile() FIBProfile {
	return p.fib
}

func (p deviceProfile) LiveProfile() LiveProfile {
	return p.live
}

func (p deviceProfile) ConfigProfile() ConfigProfile {
	return p.config
}

type interfaceProfile struct {
	kind DeviceKind
}

func (p interfaceProfile) InterfaceAliases(name string) []string {
	names := uniqueStrings(name)
	base, hasUnit := strings.CutSuffix(name, ".0")
	if hasUnit {
		names = uniqueStrings(append(names, base)...)
	}
	switch p.kind {
	case KindCEOS:
		if strings.HasPrefix(name, "eth") {
			names = uniqueStrings(append(names, "Ethernet"+strings.TrimPrefix(name, "eth"))...)
		}
	case KindSRLinux:
		switch {
		case strings.HasPrefix(name, "lo") && hasUnit:
			names = uniqueStrings(append(names, base)...)
		case strings.HasPrefix(name, "lo"):
			names = uniqueStrings(append(names, name+".0")...)
		case strings.HasPrefix(name, "e1-"):
			port := strings.TrimPrefix(name, "e1-")
			names = uniqueStrings(append(names, "ethernet-1/"+port, "ethernet-1/"+port+".0")...)
		case strings.HasPrefix(name, "ethernet-1/"):
			if hasUnit {
				names = uniqueStrings(append(names, base)...)
			} else {
				names = uniqueStrings(append(names, name+".0")...)
			}
		}
	}
	return names
}

func (p interfaceProfile) EquivalentInterfaceName(a, b string) bool {
	aliases := map[string]bool{}
	for _, alias := range p.InterfaceAliases(a) {
		aliases[alias] = true
	}
	for _, alias := range p.InterfaceAliases(b) {
		if aliases[alias] {
			return true
		}
	}
	return false
}

func (p interfaceProfile) CanonicalInterfaceName(name string) string {
	if base, ok := strings.CutSuffix(name, ".0"); ok {
		name = base
	}
	switch p.kind {
	case KindCEOS:
		if strings.HasPrefix(name, "eth") {
			return "Ethernet" + strings.TrimPrefix(name, "eth")
		}
	case KindSRLinux:
		switch {
		case strings.HasPrefix(name, "e1-"):
			return "ethernet-1/" + strings.TrimPrefix(name, "e1-")
		default:
			return name
		}
	}
	return name
}

func (p interfaceProfile) IsManagementInterface(interfaces []Interface, name string) bool {
	if name == "" {
		return false
	}
	if isKnownManagementInterface(name) {
		return true
	}
	for _, iface := range interfaces {
		if p.EquivalentInterfaceName(iface.Name, name) {
			return strings.HasPrefix(strings.ToLower(iface.Name), "mgmt")
		}
	}
	return false
}

func (p interfaceProfile) IsLoopbackInterface(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return name == "lo" || strings.HasPrefix(name, "lo") || strings.HasPrefix(name, "loopback")
}

type aclProfile struct {
	defaultAction ACLDefaultAction
}

func (p aclProfile) DefaultACLAction(fallback ACLDefaultAction) ACLDefaultAction {
	if p.defaultAction != "" {
		return p.defaultAction
	}
	return fallback
}

type fibProfile struct {
	hideLoopbackConnected bool
}

func (p fibProfile) ExpectedFIBRouteVisible(source RouteSourceKind, class ConnectedRouteClass) bool {
	if p.hideLoopbackConnected && source == RouteSourceConnected && class == ConnectedRouteClassLoopback {
		return false
	}
	return true
}

type liveProfile struct {
	supported           bool
	bgpRIBCollector     LiveCollectorID
	routeTableCollector LiveCollectorID
	fibCollector        LiveCollectorID
	requireASNForBGPRIB bool
}

func (p liveProfile) SupportsBGPRIBCollection() bool {
	return p.supported || p.bgpRIBCollector != ""
}

func (p liveProfile) SupportsRouteTableCollection() bool {
	return p.supported || p.routeTableCollector != ""
}

func (p liveProfile) SupportsFIBCollection() bool {
	return p.supported || p.fibCollector != ""
}

func (p liveProfile) BGPRIBCollector() (LiveCollectorID, bool) {
	if p.bgpRIBCollector == "" {
		return "", p.supported
	}
	return p.bgpRIBCollector, true
}

func (p liveProfile) RouteTableCollector() (LiveCollectorID, bool) {
	if p.routeTableCollector == "" {
		return "", p.supported
	}
	return p.routeTableCollector, true
}

func (p liveProfile) FIBCollector() (LiveCollectorID, bool) {
	if p.fibCollector == "" {
		return "", p.supported
	}
	return p.fibCollector, true
}

func (p liveProfile) ShouldCollectBGP(node Node) bool {
	if !p.SupportsBGPRIBCollection() {
		return false
	}
	if p.requireASNForBGPRIB && node.ASN == 0 {
		return false
	}
	return true
}

func (p liveProfile) IncludeInBGPRIBCollection(node Node) bool {
	return p.ShouldCollectBGP(node)
}

type LiveCollectorID string

const (
	LiveCollectorFRR     LiveCollectorID = "frr"
	LiveCollectorCEOS    LiveCollectorID = "ceos"
	LiveCollectorSRLinux LiveCollectorID = "srlinux"
)

type configProfile struct {
	kind             DeviceKind
	parse            bool
	ospf             bool
	routeMap         bool
	ospfVRFFromIface bool
	routeMapVendor   string
}

func (p configProfile) SupportsConfigParse() bool {
	return p.parse
}

func (p configProfile) SupportsOSPFConfig() bool {
	return p.ospf
}

func (p configProfile) SupportsRouteMapPolicy() bool {
	return p.routeMap
}

func (p configProfile) OSPFInterfaceVRF(interfaces []Interface, name string) NetworkInstanceID {
	if !p.ospfVRFFromIface {
		return NetworkInstanceDefault
	}
	for _, iface := range interfaces {
		if ProfileFor(p.kind).InterfaceProfile().EquivalentInterfaceName(iface.Name, name) {
			return NormalizeNetworkInstance(string(iface.VRF))
		}
	}
	return NetworkInstanceDefault
}

func (p configProfile) RouteMapVendorName() string {
	if p.routeMapVendor != "" {
		return p.routeMapVendor
	}
	return string(p.kind)
}

var (
	deviceProfilesMu sync.RWMutex
	deviceProfiles   = map[DeviceKind]DeviceProfile{
		KindFRR: newDeviceProfile(
			KindFRR,
			aclProfile{},
			fibProfile{},
			liveProfile{
				bgpRIBCollector:     LiveCollectorFRR,
				routeTableCollector: LiveCollectorFRR,
				fibCollector:        LiveCollectorFRR,
			},
			configProfile{kind: KindFRR, parse: true, ospf: true, routeMap: true, ospfVRFFromIface: true, routeMapVendor: "FRR"},
		),
		KindCEOS: newDeviceProfile(
			KindCEOS,
			aclProfile{defaultAction: ACLDefaultDeny},
			fibProfile{},
			liveProfile{
				bgpRIBCollector:     LiveCollectorCEOS,
				routeTableCollector: LiveCollectorCEOS,
				fibCollector:        LiveCollectorCEOS,
				requireASNForBGPRIB: true,
			},
			configProfile{kind: KindCEOS, parse: true, ospf: true, routeMap: true, routeMapVendor: "cEOS"},
		),
		KindSRLinux: newDeviceProfile(
			KindSRLinux,
			aclProfile{defaultAction: ACLDefaultDeny},
			fibProfile{hideLoopbackConnected: true},
			liveProfile{
				bgpRIBCollector:     LiveCollectorSRLinux,
				routeTableCollector: LiveCollectorSRLinux,
				fibCollector:        LiveCollectorSRLinux,
				requireASNForBGPRIB: true,
			},
			configProfile{kind: KindSRLinux, parse: true, routeMapVendor: "SR Linux"},
		),
	}
)

func newDeviceProfile(kind DeviceKind, acl ACLProfile, fib FIBProfile, live LiveProfile, config ConfigProfile) DeviceProfile {
	return deviceProfile{
		kind:   kind,
		ifaces: interfaceProfile{kind: kind},
		acl:    acl,
		fib:    fib,
		live:   live,
		config: config,
	}
}

func RegisterDeviceProfile(profile DeviceProfile) func() {
	deviceProfilesMu.Lock()
	defer deviceProfilesMu.Unlock()
	kind := profile.Kind()
	old, hadOld := deviceProfiles[kind]
	deviceProfiles[kind] = profile
	return func() {
		deviceProfilesMu.Lock()
		defer deviceProfilesMu.Unlock()
		if hadOld {
			deviceProfiles[kind] = old
			return
		}
		delete(deviceProfiles, kind)
	}
}

func ProfileFor(kind DeviceKind) DeviceProfile {
	deviceProfilesMu.RLock()
	profile, ok := deviceProfiles[kind]
	deviceProfilesMu.RUnlock()
	if ok {
		return profile
	}
	return newDeviceProfile(
		kind,
		aclProfile{},
		fibProfile{},
		liveProfile{},
		configProfile{kind: kind, routeMapVendor: string(kind)},
	)
}

func RegisteredDeviceKinds() []DeviceKind {
	deviceProfilesMu.RLock()
	defer deviceProfilesMu.RUnlock()
	out := make([]DeviceKind, 0, len(deviceProfiles))
	for kind := range deviceProfiles {
		out = append(out, kind)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i] < out[j]
	})
	return out
}

func isKnownManagementInterface(name string) bool {
	return strings.EqualFold(name, "eth0") ||
		strings.EqualFold(name, "mgmt0") ||
		strings.EqualFold(name, "Management1") ||
		strings.EqualFold(name, "mgmt")
}
