package model

import "strings"

// AFIFromPrefix determines the address family from a prefix string.
// If the prefix contains ":" it is treated as IPv6, otherwise IPv4.
func AFIFromPrefix(prefix string) AFI {
	if strings.Contains(prefix, ":") {
		return AFIIPv6
	}
	return AFIIPv4
}
