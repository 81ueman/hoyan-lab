package configparse

import (
	"fmt"
	"strconv"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func (p *frrLikeParser) handleRouterOSPF(fields []string) error {
	if !p.dialect.SupportsOSPFConfig() {
		return nil
	}
	p.currentOSPFVRF = parseFRRLikeOSPFVRF(fields)
	ospf := ospfProcess(&p.cfg, p.currentOSPFVRF)
	ospf.Enabled = true
	p.inOSPF = true
	p.inBGP = false
	p.inAF = false
	p.currentInterface = ""
	return nil
}


func (p *frrLikeParser) handleInterfaceOSPF(fields []string, line, raw string, lineNo int) error {
	if !p.dialect.SupportsOSPFConfig() || p.currentInterface == "" || len(fields) < 3 {
		return nil
	}
	kind := p.dialect.Kind()
	ospfSubCmd := fields[2]

	switch {
	case ospfSubCmd == "area" && len(fields) >= 4:
		vrf := p.dialect.OSPFInterfaceVRF(p.cfg.Interfaces, p.currentInterface)
		ospf := ospfProcess(&p.cfg, vrf)
		oi := ospfInterface(ospf, p.currentInterface)
		oi.Area = normalizeOSPFAreaID(fields[3])
		oi.Source = model.ConfigSource{Vendor: string(kind), File: p.path, Line: lineNo, Raw: line}
		ospf.Interfaces[p.currentInterface] = *oi
		ospf.Enabled = true
		return nil

	case ospfSubCmd == "cost" && len(fields) >= 4:
		cost, err := strconv.Atoi(fields[3])
		if err != nil || cost <= 0 {
			return fmt.Errorf("unsupported %s OSPF interface cost %q", p.dialect.VendorName(), line)
		}
		vrf := p.dialect.OSPFInterfaceVRF(p.cfg.Interfaces, p.currentInterface)
		ospf := ospfProcess(&p.cfg, vrf)
		oi := ospfInterface(ospf, p.currentInterface)
		oi.Cost = cost
		oi.Source = model.ConfigSource{Vendor: string(kind), File: p.path, Line: lineNo, Raw: line}
		ospf.Interfaces[p.currentInterface] = *oi
		ospf.Enabled = true
		return nil

	case ospfSubCmd == "network" && len(fields) >= 4 && isSupportedOSPFNetworkType(fields[3]):
		vrf := p.dialect.OSPFInterfaceVRF(p.cfg.Interfaces, p.currentInterface)
		ospf := ospfProcess(&p.cfg, vrf)
		oi := ospfInterface(ospf, p.currentInterface)
		oi.NetworkType = normalizeOSPFNetworkType(fields[3])
		oi.Source = model.ConfigSource{Vendor: string(kind), File: p.path, Line: lineNo, Raw: line}
		ospf.Interfaces[p.currentInterface] = *oi
		ospf.Enabled = true
		return nil

	case ospfSubCmd == "hello-interval" || ospfSubCmd == "dead-interval":
		ospfProcess(&p.cfg, p.dialect.OSPFInterfaceVRF(p.cfg.Interfaces, p.currentInterface)).Enabled = true
		return nil

	case ospfSubCmd == "mtu-ignore":
		ospfProcess(&p.cfg, p.dialect.OSPFInterfaceVRF(p.cfg.Interfaces, p.currentInterface)).Enabled = true
		return nil

	default:
		return fmt.Errorf("unsupported %s OSPF interface statement %q", p.dialect.VendorName(), line)
	}
}

// handleStaticRoute handles "ip route ..." statements.

func (p *frrLikeParser) handleOSPFRouterID(fields []string) error {
	if len(fields) < 2 {
		return nil
	}
	ospfProcess(&p.cfg, p.currentOSPFVRF).RouterID = fields[1]
	return nil
}

// handleOSPFRouterIDOrStatement handles "ospf router-id X" inside OSPF context.
func (p *frrLikeParser) handleOSPFRouterIDOrStatement(fields []string, line, raw string, lineNo int) error {
	if !p.inOSPF || len(fields) < 2 {
		return nil
	}
	// "ospf router-id X"
	if fields[1] == "router-id" && len(fields) >= 3 {
		ospfProcess(&p.cfg, p.currentOSPFVRF).RouterID = fields[2]
		return nil
	}
	// Unsupported ospf statement
	return fmt.Errorf("unsupported %s OSPF statement %q", p.dialect.VendorName(), line)
}

// handleOSPFPassiveInterface handles "passive-interface NAME" inside OSPF.
func (p *frrLikeParser) handleOSPFPassiveInterface(fields []string, line, raw string, lineNo int) error {
	if !p.inOSPF || len(fields) < 2 {
		return nil
	}
	ospf := ospfProcess(&p.cfg, p.currentOSPFVRF)
	ospf.PassiveInterfaces = appendUnique(ospf.PassiveInterfaces, fields[1])
	oi := ospfInterface(ospf, fields[1])
	oi.Passive = true
	oi.Source = model.ConfigSource{Vendor: string(p.dialect.Kind()), File: p.path, Line: lineNo, Raw: line}
	ospf.Interfaces[fields[1]] = *oi
	return nil
}

// handleOSPFArea handles "area ..." inside OSPF.
func (p *frrLikeParser) handleOSPFArea(fields []string, line, raw string, lineNo int) error {
	if !p.inOSPF || len(fields) < 3 {
		return nil
	}
	area, err := parseFRRLikeOSPFArea(p.dialect.Kind(), p.path, lineNo, raw, fields)
	if err != nil {
		return err
	}
	ospf := ospfProcess(&p.cfg, p.currentOSPFVRF)
	ospf.Areas[area.ID] = area
	return nil
}

// handleOSPFRedistribute handles "redistribute ..." inside OSPF context.
func (p *frrLikeParser) handleOSPFRedistribute(fields []string, line, raw string, lineNo int) error {
	if len(fields) < 2 {
		return nil
	}
	redist, err := parseFRRLikeOSPFRedistribution(p.dialect.Kind(), p.path, lineNo, raw, fields)
	if err != nil {
		return err
	}
	ospf := ospfProcess(&p.cfg, p.currentOSPFVRF)
	ospf.Redistribute = append(ospf.Redistribute, redist)
	return nil
}

// handleOSPFNetwork handles "network PREFIX area AREA" inside OSPF context.
func (p *frrLikeParser) handleOSPFNetwork(fields []string, line, raw string, lineNo int) error {
	if !p.dialect.SupportsOSPFConfig() || !p.inOSPF || len(fields) < 4 || fields[2] != "area" {
		return nil
	}
	prefix, err := model.ParsePrefix(fields[1])
	if err != nil {
		return fmt.Errorf("unsupported %s OSPF network %q", p.dialect.VendorName(), raw)
	}
	ospf := ospfProcess(&p.cfg, p.currentOSPFVRF)
	ospf.Networks = append(ospf.Networks, model.OSPFNetwork{
		Prefix: prefix,
		Area:   normalizeOSPFAreaID(fields[3]),
		Source: model.ConfigSource{Vendor: string(p.dialect.Kind()), File: p.path, Line: lineNo, Raw: line},
	})
	return nil
}
