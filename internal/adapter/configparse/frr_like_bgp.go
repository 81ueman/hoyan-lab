package configparse

import (
	"strconv"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func (p *frrLikeParser) handleRouterBGP(fields []string, raw string, lineNo int) error {
	if len(fields) < 3 {
		return nil
	}
	asn, err := strconv.ParseUint(fields[2], 10, 32)
	if err != nil {
		return err
	}
	p.cfg.ASN = uint32(asn)
	p.bgpVRF = model.NetworkInstanceDefault
	if len(fields) >= 5 && fields[3] == "vrf" {
		p.bgpVRF = model.NormalizeNetworkInstance(fields[4])
	}
	p.inBGP = true
	p.inAF = false
	p.inOSPF = false
	p.currentInterface = ""
	return nil
}


func (p *frrLikeParser) handleBGP(fields []string, line, raw string, lineNo int) error {
	// bgp as-path access-list (global, outside BGP context)
	if len(fields) >= 6 && p.dialect.SupportsBGPStringLists() && fields[1] == "as-path" && fields[2] == "access-list" && (fields[4] == "permit" || fields[4] == "deny") {
		addStringListRule(p.asPathLists, fields[3], model.StringListRule{Action: fields[4], Pattern: strings.Join(fields[5:], " ")})
		return nil
	}
	// bgp community-list standard (global, outside BGP context)
	if len(fields) >= 6 && p.dialect.SupportsBGPStringLists() && fields[1] == "community-list" && fields[2] == "standard" && (fields[4] == "permit" || fields[4] == "deny") {
		addCommunityListRule(p.communityLists, fields[3], model.StringListRule{Action: fields[4], Pattern: strings.Join(fields[5:], " ")})
		return nil
	}
	// "bgp router-id X" inside BGP context
	if p.inBGP && len(fields) >= 3 && fields[len(fields)-2] == "router-id" {
		p.cfg.RouterID = fields[len(fields)-1]
		return nil
	}
	return nil
}


func (p *frrLikeParser) handleBGPCommon(fields []string) error {
	if len(fields) >= 2 {
		p.cfg.RouterID = fields[1]
	}
	return nil
}

// handleBGPAddressFamily sets the address-family context.
func (p *frrLikeParser) handleBGPAddressFamily(fields []string) error {
	if !p.inBGP {
		return nil
	}
	p.inAF = true
	return nil
}

// handleBGPExitAddressFamily leaves the address-family context.
func (p *frrLikeParser) handleBGPExitAddressFamily() error {
	if !p.inBGP {
		return nil
	}
	p.inAF = false
	return nil
}

// handleBGPNeighbor handles "neighbor ..." statements under BGP.
func (p *frrLikeParser) handleBGPNeighbor(fields []string, raw string, lineNo int) error {
	if !p.inBGP || len(fields) < 3 {
		return nil
	}

	switch {
	case fields[2] == "remote-as":
		asn, err := strconv.ParseUint(fields[3], 10, 32)
		if err != nil {
			return err
		}
		n := getNeighbor(p.neighbors, p.bgpVRF, fields[1])
		n.RemoteAS = uint32(asn)
		return nil

	case p.inAF && fields[2] == "activate":
		getNeighbor(p.neighbors, p.bgpVRF, fields[1]).Activated = true
		return nil

	case p.inAF && fields[2] == "next-hop-self":
		getNeighbor(p.neighbors, p.bgpVRF, fields[1]).NextHopSelf = true
		return nil

	case p.dialect.SupportsRouteMapPolicy() && p.inAF && fields[2] == "route-map" && len(fields) >= 5:
		n := getNeighbor(p.neighbors, p.bgpVRF, fields[1])
		switch fields[4] {
		case "in":
			n.ImportPolicy = fields[3]
		case "out":
			n.ExportPolicy = fields[3]
		}
		return nil
	}
	return nil
}

// handleBGPNetwork handles "network PREFIX" within BGP address-family.
func (p *frrLikeParser) handleBGPNetwork(fields []string, line, raw string, lineNo int) error {
	if !p.inBGP || !p.inAF || len(fields) < 2 {
		return nil
	}
	kind := p.dialect.Kind()
	if p.bgpVRF == model.NetworkInstanceDefault {
		p.cfg.Prefixes = appendUnique(p.cfg.Prefixes, fields[1])
		return nil
	}
	prefix, err := model.ParsePrefix(fields[1])
	if err != nil {
		return err
	}
	p.cfg.Routes = append(p.cfg.Routes, model.ConfiguredRoute{
		NetworkInstance: p.bgpVRF,
		AFI:             model.AFIIPv4,
		Prefix:          prefix,
		Kind:            model.RouteSourceBGP,
		AdminDistance:   200,
		Source:          model.ConfigSource{Vendor: string(kind), File: p.path, Line: lineNo, Raw: line},
	})
	return nil
}

// handleBGPAggregateAddress handles "aggregate-address ..." in BGP address-family.
func (p *frrLikeParser) handleBGPAggregateAddress(fields []string, raw string, lineNo int) error {
	if !p.inBGP || !p.inAF || len(fields) < 2 {
		return nil
	}
	route, err := parseAggregateRoute(p.dialect.Kind(), p.path, lineNo, raw, fields)
	if err != nil {
		return err
	}
	route.NetworkInstance = p.bgpVRF
	p.cfg.Routes = append(p.cfg.Routes, route)
	return nil
}

// handleBGPRedistribute handles "redistribute ..." in BGP address-family.
func (p *frrLikeParser) handleBGPRedistribute(fields []string, raw string, lineNo int) error {
	if len(fields) < 2 {
		return nil
	}
	redist, err := parseFRRLikeRedistribution(p.dialect.Kind(), p.path, lineNo, raw, fields)
	if err != nil {
		return err
	}
	redist.NetworkInstance = p.bgpVRF
	p.cfg.Redistribute = append(p.cfg.Redistribute, redist)
	return nil
}

// handleOSPFRouterID handles "router-id X" inside OSPF context.
