package controlplane

import "strings"

func NormalizeArea(area string) string {
	area = strings.TrimSpace(area)
	if area == "0.0.0.0" {
		return BackboneArea
	}
	return area
}

func IsLoopbackInterface(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return name == "lo" || strings.HasPrefix(name, "lo") || strings.HasPrefix(name, "loopback")
}
