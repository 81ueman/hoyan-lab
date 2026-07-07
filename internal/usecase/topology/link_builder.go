package topology

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func buildDirectLinks(rawLinks []LabLink, topo *model.Topology, transitNodes map[string]bool) ([]model.Link, map[string][]clabTransitAttachment, error) {
	var links []model.Link
	transitAttachments := map[string][]clabTransitAttachment{}

	for i, link := range rawLinks {
		if len(link.Endpoints) != 2 {
			return nil, nil, fmt.Errorf("link %d must have two endpoints", i)
		}
		aNode, aIntf, err := splitEndpoint(link.Endpoints[0])
		if err != nil {
			return nil, nil, err
		}
		bNode, bIntf, err := splitEndpoint(link.Endpoints[1])
		if err != nil {
			return nil, nil, err
		}

		aTransit := transitNodes[aNode]
		bTransit := transitNodes[bNode]
		switch {
		case aTransit && bTransit:
			return nil, nil, fmt.Errorf("link %s-%s connects two L2 transit nodes", link.Endpoints[0], link.Endpoints[1])
		case aTransit:
			transitAttachments[aNode] = append(transitAttachments[aNode], clabTransitAttachment{Node: bNode, Intf: bIntf})
			continue
		case bTransit:
			transitAttachments[bNode] = append(transitAttachments[bNode], clabTransitAttachment{Node: aNode, Intf: aIntf})
			continue
		}

		a, _ := topo.Node(aNode)
		b, _ := topo.Node(bNode)
		subnet, err := linkSubnet(a, aIntf, b, bIntf)
		if err != nil {
			return nil, nil, fmt.Errorf("%s-%s: %w", link.Endpoints[0], link.Endpoints[1], err)
		}
		links = append(links, model.Link{
			Name:   linkName(aNode, aIntf, bNode, bIntf),
			A:      aNode,
			B:      bNode,
			AIntf:  aIntf,
			BIntf:  bIntf,
			Cost:   1,
			Subnet: subnet.String(),
		})
	}

	return links, transitAttachments, nil
}

func splitEndpoint(endpoint string) (string, string, error) {
	parts := strings.Split(endpoint, ":")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid endpoint %q", endpoint)
	}
	return parts[0], parts[1], nil
}

func linkSubnet(a model.Node, aIntf string, b model.Node, bIntf string) (netip.Prefix, error) {
	ap, aok := model.InterfaceAddress(a.Kind, a.Interfaces, aIntf)
	bp, bok := model.InterfaceAddress(b.Kind, b.Interfaces, bIntf)
	switch {
	case aok && bok && ap.Masked() == bp.Masked():
		return ap.Masked(), nil
	case aok && !bok:
		return ap.Masked(), nil
	case !aok && bok:
		return bp.Masked(), nil
	case aok && bok:
		return netip.Prefix{}, fmt.Errorf("interface subnets differ: %s and %s", ap, bp)
	default:
		return netip.Prefix{}, fmt.Errorf("missing interface addresses")
	}
}

func linkName(aNode, aIntf, bNode, bIntf string) string {
	return strings.NewReplacer(":", "-", "_", "-").Replace(aNode + "-" + aIntf + "__" + bNode + "-" + bIntf)
}
