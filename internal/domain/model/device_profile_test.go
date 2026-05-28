package model

import "testing"

func TestDeviceProfileVendorSemantics(t *testing.T) {
	tests := []struct {
		name             string
		kind             DeviceKind
		defaultACL       ACLDefaultAction
		bgpNodeASN       uint32
		wantBGPLive      bool
		loopbackFIBRoute bool
	}{
		{
			name:             "frr",
			kind:             KindFRR,
			defaultACL:       ACLDefaultPermit,
			wantBGPLive:      true,
			loopbackFIBRoute: true,
		},
		{
			name:             "ceos",
			kind:             KindCEOS,
			defaultACL:       ACLDefaultDeny,
			bgpNodeASN:       65000,
			wantBGPLive:      true,
			loopbackFIBRoute: true,
		},
		{
			name:             "srlinux",
			kind:             KindSRLinux,
			defaultACL:       ACLDefaultDeny,
			bgpNodeASN:       65000,
			wantBGPLive:      true,
			loopbackFIBRoute: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := ProfileFor(tt.kind)
			if got := profile.ACLProfile().DefaultACLAction(ACLDefaultPermit); got != tt.defaultACL {
				t.Fatalf("default ACL action = %s, want %s", got, tt.defaultACL)
			}
			node := Node{Name: "r1", Kind: tt.kind, ASN: tt.bgpNodeASN}
			if got := profile.LiveProfile().IncludeInBGPRIBCollection(node); got != tt.wantBGPLive {
				t.Fatalf("IncludeInBGPRIBCollection = %v, want %v", got, tt.wantBGPLive)
			}
			gotFIB := profile.FIBProfile().ExpectedFIBRouteVisible(RouteSourceConnected, ConnectedRouteClassLoopback)
			if gotFIB != tt.loopbackFIBRoute {
				t.Fatalf("ExpectedFIBRouteVisible(loopback connected) = %v, want %v", gotFIB, tt.loopbackFIBRoute)
			}
		})
	}
}

func TestDeviceProfileRegistrationRestoresPreviousProfile(t *testing.T) {
	const kind DeviceKind = "test-kind"
	restore := RegisterDeviceProfile(newDeviceProfile(
		kind,
		aclProfile{defaultAction: ACLDefaultDeny},
		fibProfile{},
		liveProfile{supported: true},
		configProfile{kind: kind, routeMapVendor: "test"},
	))
	if got := ProfileFor(kind).ACLProfile().DefaultACLAction(ACLDefaultPermit); got != ACLDefaultDeny {
		t.Fatalf("registered ACL action = %s, want %s", got, ACLDefaultDeny)
	}
	restore()
	if got := ProfileFor(kind).LiveProfile().SupportsFIBCollection(); got {
		t.Fatalf("restored unknown profile SupportsFIBCollection = %v, want false", got)
	}
}

func TestLiveProfileCollectorRegistry(t *testing.T) {
	tests := []struct {
		kind          DeviceKind
		wantCollector LiveCollectorID
	}{
		{kind: KindFRR, wantCollector: LiveCollectorFRR},
		{kind: KindCEOS, wantCollector: LiveCollectorCEOS},
		{kind: KindSRLinux, wantCollector: LiveCollectorSRLinux},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			live := ProfileFor(tt.kind).LiveProfile()
			if got, ok := live.BGPRIBCollector(); !ok || got != tt.wantCollector {
				t.Fatalf("BGPRIBCollector = %q, %v; want %q, true", got, ok, tt.wantCollector)
			}
			if got, ok := live.RouteTableCollector(); !ok || got != tt.wantCollector {
				t.Fatalf("RouteTableCollector = %q, %v; want %q, true", got, ok, tt.wantCollector)
			}
			if got, ok := live.FIBCollector(); !ok || got != tt.wantCollector {
				t.Fatalf("FIBCollector = %q, %v; want %q, true", got, ok, tt.wantCollector)
			}
		})
	}
}
