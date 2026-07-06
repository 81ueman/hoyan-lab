package configparse

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func ospfProcess(cfg *ParsedConfig, vrf model.NetworkInstanceID) *model.OSPFProcess {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	if vrf == model.NetworkInstanceDefault {
		cfg.OSPF.NetworkInstance = model.NetworkInstanceDefault
		if cfg.OSPF.Interfaces == nil {
			cfg.OSPF.Interfaces = map[string]model.OSPFInterface{}
		}
		if cfg.OSPF.Areas == nil {
			cfg.OSPF.Areas = map[string]model.OSPFArea{}
		}
		return &cfg.OSPF
	}
	for i := range cfg.OSPFProcesses {
		if model.NormalizeNetworkInstance(string(cfg.OSPFProcesses[i].NetworkInstance)) == vrf {
			if cfg.OSPFProcesses[i].Interfaces == nil {
				cfg.OSPFProcesses[i].Interfaces = map[string]model.OSPFInterface{}
			}
			if cfg.OSPFProcesses[i].Areas == nil {
				cfg.OSPFProcesses[i].Areas = map[string]model.OSPFArea{}
			}
			return &cfg.OSPFProcesses[i]
		}
	}
	cfg.OSPFProcesses = append(cfg.OSPFProcesses, model.OSPFProcess{
		NetworkInstance: vrf,
		Interfaces:      map[string]model.OSPFInterface{},
		Areas:           map[string]model.OSPFArea{},
	})
	return &cfg.OSPFProcesses[len(cfg.OSPFProcesses)-1]
}

func ospfInterface(ospf *model.OSPFProcess, name string) *model.OSPFInterface {
	if ospf.Interfaces == nil {
		ospf.Interfaces = map[string]model.OSPFInterface{}
	}
	oi := ospf.Interfaces[name]
	if oi.Name == "" {
		oi.Name = name
	}
	ospf.Interfaces[name] = oi
	return &oi
}

func parseFRRLikeOSPFVRF(fields []string) model.NetworkInstanceID {
	for i := 2; i+1 < len(fields); i++ {
		if fields[i] == "vrf" {
			return model.NormalizeNetworkInstance(fields[i+1])
		}
	}
	return model.NetworkInstanceDefault
}

func compactOSPFProcesses(processes []model.OSPFProcess) []model.OSPFProcess {
	out := processes[:0]
	for _, process := range processes {
		process.NetworkInstance = model.NormalizeNetworkInstance(string(process.NetworkInstance))
		if process.NetworkInstance == model.NetworkInstanceDefault || !process.Enabled {
			continue
		}
		out = append(out, process)
	}
	return out
}

func parseFRRLikeOSPFArea(kind model.DeviceKind, path string, lineNo int, raw string, fields []string) (model.OSPFArea, error) {
	area := model.OSPFArea{ID: normalizeOSPFAreaID(fields[1]), Source: model.ConfigSource{Vendor: string(kind), File: path, Line: lineNo, Raw: raw}}
	switch fields[2] {
	case "stub":
		area.Kind = model.OSPFAreaStub
	case "nssa":
		area.Kind = model.OSPFAreaNSSA
	default:
		return model.OSPFArea{}, fmt.Errorf("unsupported %s OSPF area statement", routeMapVendorName(kind))
	}
	for _, opt := range fields[3:] {
		switch opt {
		case "no-summary":
			area.NoSummary = true
		case "default-information-originate":
			if area.Kind != model.OSPFAreaNSSA {
				return model.OSPFArea{}, fmt.Errorf("unsupported %s OSPF area option %q", routeMapVendorName(kind), opt)
			}
			area.DefaultInformationOriginate = true
		default:
			return model.OSPFArea{}, fmt.Errorf("unsupported %s OSPF area option %q", routeMapVendorName(kind), opt)
		}
	}
	return area, nil
}

func parseFRRLikeOSPFRedistribution(kind model.DeviceKind, path string, lineNo int, raw string, fields []string) (model.OSPFRedistribution, error) {
	if len(fields) < 2 {
		return model.OSPFRedistribution{}, fmt.Errorf("unsupported %s OSPF redistribute statement", routeMapVendorName(kind))
	}
	redist := model.OSPFRedistribution{MetricType: 2, Source: model.ConfigSource{Vendor: string(kind), File: path, Line: lineNo, Raw: raw}}
	switch fields[1] {
	case "connected":
		redist.Kind = model.RouteSourceConnected
	case "static":
		redist.Kind = model.RouteSourceStatic
	case "bgp":
		redist.Kind = model.RouteSourceBGP
	default:
		return model.OSPFRedistribution{}, fmt.Errorf("unsupported %s OSPF redistribute source %q", routeMapVendorName(kind), fields[1])
	}
	for i := 2; i < len(fields); {
		switch fields[i] {
		case "route-map":
			if i+1 >= len(fields) {
				return model.OSPFRedistribution{}, fmt.Errorf("unsupported %s OSPF redistribute statement", routeMapVendorName(kind))
			}
			redist.RouteMap = fields[i+1]
			i += 2
		case "metric":
			if i+1 >= len(fields) {
				return model.OSPFRedistribution{}, fmt.Errorf("unsupported %s OSPF redistribute metric", routeMapVendorName(kind))
			}
			metric, err := strconv.Atoi(fields[i+1])
			if err != nil || metric < 0 {
				return model.OSPFRedistribution{}, fmt.Errorf("unsupported %s OSPF redistribute metric", routeMapVendorName(kind))
			}
			redist.Metric = metric
			i += 2
		case "metric-type":
			if i+1 >= len(fields) {
				return model.OSPFRedistribution{}, fmt.Errorf("unsupported %s OSPF redistribute metric-type", routeMapVendorName(kind))
			}
			metricType, err := strconv.Atoi(fields[i+1])
			if err != nil || (metricType != 1 && metricType != 2) {
				return model.OSPFRedistribution{}, fmt.Errorf("unsupported %s OSPF redistribute metric-type", routeMapVendorName(kind))
			}
			redist.MetricType = metricType
			i += 2
		default:
			return model.OSPFRedistribution{}, fmt.Errorf("unsupported %s OSPF redistribute option %q", routeMapVendorName(kind), fields[i])
		}
	}
	return redist, nil
}

func parseSRLinuxOSPF(cfg *ParsedConfig, path string, lineNo int, raw string, fields []string) error {
	ospf := ospfProcess(cfg, model.NetworkInstanceDefault)
	ospf.Enabled = true
	source := model.ConfigSource{Vendor: string(model.KindSRLinux), File: path, Line: lineNo, Raw: raw}
	if containsAnyField(fields, "router-id") {
		ospf.RouterID = fields[len(fields)-1]
		return nil
	}
	if containsSeq(fields, "area", "interface") {
		iface := fieldAfter(fields, "interface")
		area := normalizeOSPFAreaID(fieldAfter(fields, "area"))
		if iface == "" || area == "" {
			return fmt.Errorf("unsupported SR Linux OSPF interface statement")
		}
		oi := ospfInterface(ospf, iface)
		oi.Area = area
		oi.Source = source
		if containsAnyField(fields, "metric") {
			cost, err := strconv.Atoi(fields[len(fields)-1])
			if err != nil || cost <= 0 {
				return fmt.Errorf("unsupported SR Linux OSPF interface metric")
			}
			oi.Cost = cost
		}
		if containsAnyField(fields, "interface-type") {
			networkType := normalizeOSPFNetworkType(fields[len(fields)-1])
			if !isSupportedOSPFNetworkType(networkType) {
				return fmt.Errorf("unsupported SR Linux OSPF interface type")
			}
			oi.NetworkType = networkType
		}
		if containsAnyField(fields, "passive") && parseConfigBool(fields[len(fields)-1]) {
			oi.Passive = true
			ospf.PassiveInterfaces = appendUnique(ospf.PassiveInterfaces, iface)
		}
		ospf.Interfaces[iface] = *oi
		return nil
	}
	if containsAnyField(fields, "admin-state", "version") {
		return nil
	}
	return fmt.Errorf("unsupported SR Linux OSPF statement")
}

func isSupportedOSPFNetworkType(raw string) bool {
	switch normalizeOSPFNetworkType(raw) {
	case "", "broadcast", "point-to-point":
		return true
	default:
		return false
	}
}

func normalizeOSPFNetworkType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "point-to-point", "p2p":
		return "point-to-point"
	case "broadcast":
		return "broadcast"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func normalizeOSPFAreaID(area string) string {
	switch strings.TrimSpace(area) {
	case "0.0.0.0":
		return "0"
	default:
		return strings.TrimSpace(area)
	}
}

func parseConfigBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "yes", "enable", "enabled":
		return true
	default:
		return false
	}
}
