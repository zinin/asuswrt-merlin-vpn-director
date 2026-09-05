package vpnconfig

import (
	"errors"
	"net"
	"strings"
)

// ErrInvalidClientAddr is returned by NormalizeClientAddr for anything that
// is not an IPv4 address or an IPv4 CIDR.
var ErrInvalidClientAddr = errors.New("invalid IPv4 address or CIDR")

// NormalizeClientAddr trims s, accepts an IPv4 address or an IPv4 CIDR,
// strips a trailing /32 and returns the canonical string. It is the single
// validator for LAN client addresses and exclude IPs, shared by the bot and
// the Web UI so both persist the same form. Router ipsets are IPv4-only, so
// every IPv6 spelling (including IPv4-mapped) is rejected.
func NormalizeClientAddr(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || strings.Contains(s, ":") {
		return "", ErrInvalidClientAddr
	}
	if strings.Contains(s, "/") {
		ip, _, err := net.ParseCIDR(s)
		if err != nil || ip.To4() == nil {
			return "", ErrInvalidClientAddr
		}
		return strings.TrimSuffix(s, "/32"), nil
	}
	ip := net.ParseIP(s)
	if ip == nil || ip.To4() == nil {
		return "", ErrInvalidClientAddr
	}
	return ip.To4().String(), nil
}
