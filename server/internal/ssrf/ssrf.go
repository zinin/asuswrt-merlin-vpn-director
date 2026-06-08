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

// IPv6 transition prefixes that embed an IPv4 address; the embedded IPv4 must be
// unwrapped and re-checked so it cannot smuggle a private destination past the
// guard. IPv4-mapped (::ffff:0:0/96) is intentionally absent — net.IP.To4()
// already normalizes it, so the predicates handle it directly.
var (
	nat64WellKnown = mustCIDR("64:ff9b::/96") // RFC 6052 NAT64 well-known prefix
	sixToFour      = mustCIDR("2002::/16")    // RFC 3056 6to4
)

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
// have no stdlib predicate (CGNAT, "this network"). Finally it unwraps the
// IPv4-embedding IPv6 transition forms (NAT64, 6to4, IPv4-compatible) and
// re-checks the embedded IPv4, which the predicates do not classify.
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
	// Unwrap IPv4-embedding IPv6 transition addresses and re-check the embedded
	// IPv4, so a literal like 64:ff9b::7f00:1 (NAT64-wrapped 127.0.0.1) cannot
	// bypass the guard on a host with NAT64/6to4 routing.
	if v4 := embeddedIPv4(ip); v4 != nil {
		return IsPrivateOrReserved(v4)
	}
	return false
}

// embeddedIPv4 returns the IPv4 address embedded in an IPv6 transition address
// (NAT64 well-known prefix, 6to4, or the deprecated IPv4-compatible form), or
// nil for any other address. IPv4 and IPv4-mapped IPv6 return nil (To4() != nil)
// because the net.IP predicates already handle them. The returned IPv4 is itself
// re-checked by IsPrivateOrReserved, so no transition encoding can hide a private
// destination.
func embeddedIPv4(ip net.IP) net.IP {
	if len(ip) != net.IPv6len || ip.To4() != nil {
		return nil
	}
	switch {
	case nat64WellKnown.Contains(ip): // 64:ff9b::/96 -> low 32 bits
		return net.IPv4(ip[12], ip[13], ip[14], ip[15])
	case sixToFour.Contains(ip): // 2002::/16 -> bits 16..48
		return net.IPv4(ip[2], ip[3], ip[4], ip[5])
	case isZeros(ip[:12]): // ::/96 IPv4-compatible (deprecated) -> low 32 bits
		return net.IPv4(ip[12], ip[13], ip[14], ip[15])
	}
	return nil
}

func isZeros(b []byte) bool {
	for _, x := range b {
		if x != 0 {
			return false
		}
	}
	return true
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
	// Client.Timeout bounds the whole request, so the connect/TLS sub-phase
	// budgets only need to be capped at it (never larger). ResponseHeaderTimeout
	// is intentionally omitted: equal to the overall timeout it is redundant, and
	// any value would just starve the body read once headers arrive at deadline.
	connectTimeout := timeout
	if connectTimeout > 10*time.Second {
		connectTimeout = 10 * time.Second
	}
	dialer := &net.Dialer{
		Timeout: connectTimeout,
		Control: dialGuard,
	}
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			DialContext:         dialer.DialContext,
			TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
			TLSHandshakeTimeout: connectTimeout,
		},
	}
}
