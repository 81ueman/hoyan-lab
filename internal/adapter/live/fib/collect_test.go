package fib

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

type fakeRunner struct {
	fn func(name string, args ...string) ([]byte, error)
}

func (f fakeRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if f.fn == nil {
		return nil, errors.New("unexpected command")
	}
	return f.fn(name, args...)
}

func TestCollectRejectsUnsupportedNodes(t *testing.T) {
	_, err := CollectFIB(context.Background(), fakeRunner{}, model.Node{Name: "unknown1", Kind: model.DeviceKind("unknown")}, model.NetworkInstanceDefault, observation.Options{})
	if err == nil || !strings.Contains(err.Error(), "unsupported live FIB collector") {
		t.Fatalf("CollectFIB() error = %v", err)
	}
}

func TestCollectFIBAFICommandSelectionFRR(t *testing.T) {
	tests := []struct {
		name   string
		afi    model.AFI
		wantV4 bool
		wantV6 bool
	}{
		{name: "default (empty) AFI uses IPv4", afi: "", wantV4: true, wantV6: false},
		{name: "IPv4 AFI uses IPv4 commands", afi: model.AFIIPv4, wantV4: true, wantV6: false},
		{name: "IPv6 AFI uses IPv6 commands", afi: model.AFIIPv6, wantV4: false, wantV6: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seenV4 := false
			seenV6 := false
			runner := fakeRunner{fn: func(name string, args ...string) ([]byte, error) {
				cmd := name + " " + strings.Join(args, " ")
				if strings.Contains(cmd, "ip -j route show") && !strings.Contains(cmd, "-6") {
					seenV4 = true
				}
				if strings.Contains(cmd, "ip -j -6 route show") {
					seenV6 = true
				}
				return []byte(`[]`), nil
			}}
			opts := observation.Options{AFI: tt.afi}
			_, err := CollectFIB(context.Background(), runner, model.Node{Name: "frr", Kind: model.KindFRR}, model.NetworkInstanceDefault, opts)
			if err != nil {
				t.Fatalf("CollectFIB() error = %v", err)
			}
			if seenV4 != tt.wantV4 {
				t.Errorf("IPv4 command seen = %v, want %v", seenV4, tt.wantV4)
			}
			if seenV6 != tt.wantV6 {
				t.Errorf("IPv6 command seen = %v, want %v", seenV6, tt.wantV6)
			}
		})
	}
}

func TestCollectFIBAFICommandSelectionSRLinux(t *testing.T) {
	tests := []struct {
		name     string
		afi      model.AFI
		wantIPv4 bool
		wantIPv6 bool
	}{
		{name: "default AFI uses ipv4-unicast", afi: "", wantIPv4: true, wantIPv6: false},
		{name: "IPv4 AFI uses ipv4-unicast", afi: model.AFIIPv4, wantIPv4: true, wantIPv6: false},
		{name: "IPv6 AFI uses ipv6-unicast", afi: model.AFIIPv6, wantIPv4: false, wantIPv6: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seenIPv4 := false
			seenIPv6 := false
			runner := fakeRunner{fn: func(name string, args ...string) ([]byte, error) {
				cmd := name + " " + strings.Join(args, " ")
				if strings.Contains(cmd, "route-table ipv4-unicast") {
					seenIPv4 = true
				}
				if strings.Contains(cmd, "route-table ipv6-unicast") {
					seenIPv6 = true
				}
				return []byte(`{"instance":[{"ip route":[]}]}`), nil
			}}
			opts := observation.Options{AFI: tt.afi}
			_, err := CollectFIB(context.Background(), runner, model.Node{Name: "srl", Kind: model.KindSRLinux}, model.NetworkInstanceDefault, opts)
			if err != nil {
				t.Fatalf("CollectFIB() error = %v", err)
			}
			if seenIPv4 != tt.wantIPv4 {
				t.Errorf("IPv4 command seen = %v, want %v", seenIPv4, tt.wantIPv4)
			}
			if seenIPv6 != tt.wantIPv6 {
				t.Errorf("IPv6 command seen = %v, want %v", seenIPv6, tt.wantIPv6)
			}
		})
	}
}

func TestCollectFRRKernelRoutes(t *testing.T) {
	runner := fakeRunner{fn: func(name string, args ...string) ([]byte, error) {
		got := name + " " + strings.Join(args, " ")
		switch got {
		case "docker exec -i r1 ip -j route show table main":
			return []byte(`[{"dst":"10.0.0.0/24","gateway":"192.0.2.1","dev":"eth1","protocol":"bgp"}]`), nil
		case "docker exec -i r1 ip -j route show table local":
			return []byte(`[]`), nil
		default:
			return nil, errors.New("unexpected command: " + got)
		}
	}}
	fib, err := CollectFIB(context.Background(), runner, model.Node{Name: "r1", Kind: model.KindFRR}, model.NetworkInstanceDefault, observation.Options{})
	if err != nil {
		t.Fatalf("CollectFIB() error = %v", err)
	}
	entries := fib.Entries
	if len(entries) != 1 || entries[0].Prefix != "10.0.0.0/24" {
		t.Fatalf("fib = %#v", fib)
	}
}

func TestCollectAllSupportedKinds(t *testing.T) {
	runner := fakeRunner{fn: func(name string, args ...string) ([]byte, error) {
		cmd := name + " " + strings.Join(args, " ")
		switch {
		case cmd == "docker exec -i frr ip -j route show table main":
			return []byte(`[{"dst":"10.0.0.0/24","gateway":"192.0.2.1","dev":"eth1","protocol":"bgp"}]`), nil
		case cmd == "docker exec -i frr ip -j route show table local":
			return []byte(`[]`), nil
		case cmd == "docker exec -i ceos Cli -p 15 -c show ip route vrf all | json":
			return []byte(`{"vrfs":{"default":{"routes":{"10.0.1.0/24":{"kernelProgrammed":true,"routeType":"eBGP","vias":[{"nexthopAddr":"192.0.2.2","interface":"Ethernet1"}]}}}}}`), nil
		case cmd == "docker exec -i srl sr_cli --output-format json --pagination off -- show network-instance default route-table ipv4-unicast summary":
			return []byte(`{"instance":[{"ip route":[{"Prefix":"10.0.2.0/24","Route Type":"bgp","Active":"True","Next-hop (Type)":"192.0.2.3/31 (indirect/local)","Next-hop Interface":"ethernet-1/1.0 "}]}]}`), nil
		case cmd == "docker exec -i srl sr_cli --output-format json --pagination off -- show network-instance default route-table ipv4-unicast prefix 10.0.2.0/24 detail":
			return []byte(`{"instance":[{"ip route":[{"Destination":"10.0.2.0/24","Route Type":"bgp","Active":true,"ip route nexthop":{"Next hops":"192.0.2.2 (indirect) resolved by route to 192.0.2.3/31 (local)\n  via 192.0.2.2 (direct) via [ethernet-1/1.0]"}}]}]}`), nil
		default:
			return nil, errors.New("unexpected command: " + cmd)
		}
	}}
	var fibs []FIB
	for _, node := range []model.Node{
		{Name: "frr", Kind: model.KindFRR},
		{Name: "ceos", Kind: model.KindCEOS},
		{Name: "srl", Kind: model.KindSRLinux},
	} {
		fib, err := CollectFIB(context.Background(), runner, node, model.NetworkInstanceDefault, observation.Options{})
		if err != nil {
			t.Fatalf("CollectFIB() error = %v", err)
		}
		fibs = append(fibs, fib)
	}
	for _, prefix := range []string{"10.0.0.0/24", "10.0.1.0/24", "10.0.2.0/24"} {
		if routeByPrefix(flattenTestFIBs(fibs), prefix) == nil {
			t.Fatalf("fibs missing %s: %#v", prefix, fibs)
		}
	}
}

func TestCollectSRLinuxUsesRouteDetailPeerGateway(t *testing.T) {
	resolve := func(name string) string {
		if name == "core-gz" {
			return "srl1"
		}
		return name
	}
	runner := fakeRunner{fn: func(name string, args ...string) ([]byte, error) {
		cmd := name + " " + strings.Join(args, " ")
		switch cmd {
		case "docker exec -i srl1 sr_cli --output-format json --pagination off -- show network-instance default route-table ipv4-unicast summary":
			return []byte(`{"instance":[{"ip route":[
			  {"Prefix":"10.4.0.0/16","Route Type":"bgp","Active":"True","Metric":0,"Pref":170,"Next-hop (Type)":"198.18.20.4/31 (indirect/local)","Next-hop Interface":"ethernet-1/4.0 "},
			  {"Prefix":"198.18.20.4/31","Route Type":"local","Active":"True","Next-hop (Type)":"198.18.20.4 (direct)","Next-hop Interface":"ethernet-1/4.0 "}
			]}]}`), nil
		case "docker exec -i srl1 sr_cli --output-format json --pagination off -- show network-instance default route-table ipv4-unicast prefix 10.4.0.0/16 detail":
			return []byte(`{"instance":[{"ip route":[{"Destination":"10.4.0.0/16","Route Type":"bgp","Active":true,"Preference":170,"ip route nexthop":{"Next Hop Count":1,"Next hops":"198.18.20.5 (indirect) resolved by route to 198.18.20.4/31 (local)\n  via 198.18.20.5 (direct) via [ethernet-1/4.0]"}}]}]}`), nil
		default:
			return nil, errors.New("unexpected command: " + cmd)
		}
	}}
	fib, err := NewCollectorWithResolver(runner, resolve).CollectFIB(context.Background(), model.Node{Name: "core-gz", Kind: model.KindSRLinux}, model.NetworkInstanceDefault, observation.Options{})
	if err != nil {
		t.Fatalf("CollectFIB() error = %v", err)
	}
	route := routeByPrefix(fib.Entries, "10.4.0.0/16")
	if route == nil {
		t.Fatalf("fib = %#v", fib)
	}
	want := []observation.NextHop{{Address: "198.18.20.5", Interface: "ethernet-1/4.0"}}
	if !reflect.DeepEqual(route.NextHops, want) {
		t.Fatalf("next-hops = %#v, want %#v", route.NextHops, want)
	}
}

func TestCollectSRLinuxFallsBackToTTYWhenJSONIsEmpty(t *testing.T) {
	resolve := func(name string) string {
		if name == "core-gz" {
			return "srl1"
		}
		return name
	}
	runner := fakeRunner{fn: func(name string, args ...string) ([]byte, error) {
		cmd := name + " " + strings.Join(args, " ")
		switch {
		case cmd == "docker exec -i srl1 sr_cli --output-format json --pagination off -- show network-instance default route-table ipv4-unicast summary":
			return []byte{}, nil
		case strings.HasPrefix(cmd, "script -q /dev/null -c docker exec -it 'srl1' 'sr_cli' '--output-format' 'json' '--pagination' 'off' '--' 'show' 'network-instance' 'default' 'route-table' 'ipv4-unicast' 'summary'"):
			return []byte(`{"instance":[{"ip route":[{"Prefix":"198.18.20.4/31","Route Type":"local","Active":"True","Next-hop (Type)":"198.18.20.4 (direct)","Next-hop Interface":"ethernet-1/4.0 "}]}]}`), nil
		default:
			return nil, errors.New("unexpected command: " + cmd)
		}
	}}
	fib, err := NewCollectorWithResolver(runner, resolve).CollectFIB(context.Background(), model.Node{Name: "core-gz", Kind: model.KindSRLinux}, model.NetworkInstanceDefault, observation.Options{})
	if err != nil {
		t.Fatalf("CollectFIB() error = %v", err)
	}
	if routeByPrefix(fib.Entries, "198.18.20.4/31") == nil {
		t.Fatalf("fib = %#v", fib)
	}
}

func flattenTestFIBs(fibs []FIB) []FIBEntry {
	var out []FIBEntry
	for _, fib := range fibs {
		out = append(out, fib.Entries...)
	}
	return out
}
