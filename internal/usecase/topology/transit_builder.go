package topology

import (
	"fmt"
	"sort"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

type clabTransitAttachment struct {
	Node string
	Intf string
}

func isL2TransitNode(n LabNode) bool {
	group := strings.ToLower(strings.TrimSpace(n.Group))
	kind := strings.ToLower(strings.TrimSpace(n.Kind))
	mode := strings.ToLower(strings.TrimSpace(n.NetworkMode))
	return group == "switch" || group == "l2" || kind == "bridge" || mode == "bridge"
}

func expandTransitLinks(transitAttachments map[string][]clabTransitAttachment, topo *model.Topology) ([]model.Link, error) {
	var links []model.Link
	segments := make([]string, 0, len(transitAttachments))
	for segment := range transitAttachments {
		segments = append(segments, segment)
	}
	sort.Strings(segments)

	for _, segment := range segments {
		segmentLinks, err := expandTransitSegment(segment, transitAttachments[segment], topo)
		if err != nil {
			return nil, err
		}
		links = append(links, segmentLinks...)
	}
	return links, nil
}

func expandTransitSegment(segment string, attachments []clabTransitAttachment, topo *model.Topology) ([]model.Link, error) {
	if len(attachments) < 2 {
		return nil, fmt.Errorf("L2 transit node %s has fewer than two router attachments", segment)
	}
	sort.Slice(attachments, func(i, j int) bool {
		if attachments[i].Node == attachments[j].Node {
			return attachments[i].Intf < attachments[j].Intf
		}
		return attachments[i].Node < attachments[j].Node
	})

	var links []model.Link
	for i := 0; i < len(attachments); i++ {
		for j := i + 1; j < len(attachments); j++ {
			aRef := attachments[i]
			bRef := attachments[j]
			a, _ := topo.Node(aRef.Node)
			b, _ := topo.Node(bRef.Node)
			subnet, err := linkSubnet(a, aRef.Intf, b, bRef.Intf)
			if err != nil {
				return nil, fmt.Errorf("%s:%s-%s:%s via %s: %w", aRef.Node, aRef.Intf, bRef.Node, bRef.Intf, segment, err)
			}
			links = append(links, model.Link{
				Name:   linkName(segment, aRef.Node+"-"+aRef.Intf, bRef.Node, bRef.Intf),
				A:      aRef.Node,
				B:      bRef.Node,
				AIntf:  aRef.Intf,
				BIntf:  bRef.Intf,
				Cost:   1,
				Subnet: subnet.String(),
			})
		}
	}
	return links, nil
}
