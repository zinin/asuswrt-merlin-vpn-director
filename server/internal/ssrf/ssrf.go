// Package ssrf provides an HTTP client hardened against Server-Side Request
// Forgery. The authoritative protection validates the resolved connection IP at
// dial time via net.Dialer.Control, which closes the DNS-rebinding / TOCTOU gap
// that hostname-based checks miss.
package ssrf

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// ErrBlockedAddress is returned by the dial guard when a connection targets a
// private, loopback, link-local, multicast, CGNAT, or unspecified address.
var ErrBlockedAddress = errors.New("connection to a private or reserved address is blocked")

// extraBlockedCIDRs covers reserved ranges that no net.IP predicate reports.
var extraBlockedCIDRs = []*net.IPNet{
	mustCIDR("100.64.0.0/10"), // RFC 6598 Carrier-Grade NAT
	mustCIDR("0.0.0.0/8"),     // RFC 1122 "this network"
}

func mustCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic("ssrf: bad CIDR " + s + ": " + err.Error())
	}
	return n
}

// IsPrivateOrReserved reports whether ip must not be reachable from a
// user-supplied URL. It combines the disjoint net.IP predicates — each calls
// To4() internally, so IPv4-mapped IPv6 (e.g. ::ffff:127.0.0.1) is normalized
// and cannot be used to bypass the check — plus the extra reserved CIDRs that
// have no stdlib predicate (CGNAT, "this network").
func IsPrivateOrReserved(ip net.IP) bool {
	if ip == nil {
		return true // fail closed on unparseable input
	}
	if ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified() {
		return true
	}
	for _, n := range extraBlockedCIDRs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// IsPrivateHost reports whether host (a hostname or IP literal) resolves to any
// private/reserved address. It is a fast-fail pre-flight check; the dial-time
// guard installed by NewClient is the authoritative protection against DNS
// rebinding. Unresolvable hosts fail closed (reported as private).
func IsPrivateHost(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		return IsPrivateOrReserved(ip)
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return true // fail closed
	}
	for _, ip := range ips {
		if IsPrivateOrReserved(ip) {
			return true
		}
	}
	return false
}

// dialGuard is a net.Dialer.Control hook. It runs after DNS resolution but
// before connect, so address is the concrete resolved IP:port — checking it
// here (rather than the hostname) defeats DNS-rebinding / TOCTOU attacks.
func dialGuard(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("ssrf: parse dial address %q: %w", address, err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("ssrf: unparseable dial IP %q", host)
	}
	if IsPrivateOrReserved(ip) {
		return fmt.Errorf("%w: %s", ErrBlockedAddress, ip)
	}
	return nil
}

// NewClient returns an *http.Client hardened against SSRF: it validates the
// resolved IP at dial time and refuses to follow redirects (so a public URL
// cannot redirect into the internal network). timeout bounds the whole request.
func NewClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: dialGuard,
	}
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			DialContext:           dialer.DialContext,
			TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: timeout,
		},
	}
}
