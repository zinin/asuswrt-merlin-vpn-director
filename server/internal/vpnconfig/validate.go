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
		ip, ipnet, err := net.ParseCIDR(s)
		// The ip.To4() guard is unreachable today: the ":" fast path above
		// rejects every IPv6 spelling, and ParseCIDR on IPv4 always yields a
		// non-nil To4(). Kept as defence in depth for the day that fast path
		// changes.
		if err != nil || ip.To4() == nil {
			return "", ErrInvalidClientAddr
		}
		// ParseCIDR already masked the host bits, so returning the parsed
		// network canonicalizes "192.168.50.10/24" and zero-padded prefixes.
		return strings.TrimSuffix(ipnet.String(), "/32"), nil
	}
	ip := net.ParseIP(s)
	if ip == nil || ip.To4() == nil {
		return "", ErrInvalidClientAddr
	}
	return ip.To4().String(), nil
}
