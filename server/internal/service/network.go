// internal/service/network.go
package service

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

// ExternalIPTimeout bounds the curl call behind /ip and the Status tab. curl
// has its own --max-time 10; this is the outer safety net.
const ExternalIPTimeout = 15 * time.Second

// curlErrors names the exit codes this one call can realistically produce.
// The Web UI shows whatever comes back, and a bare number told nobody which
// half of the request had failed.
var curlErrors = map[int]string{
	6:  "could not resolve host",
	7:  "could not connect",
	28: "timed out",
}

// NetworkService handles network-related operations
type NetworkService struct {
	executor ShellExecutor
}

// Compile-time check that NetworkService implements NetworkInfo
var _ NetworkInfo = (*NetworkService)(nil)

// NewNetworkService creates a new NetworkService
func NewNetworkService(executor ShellExecutor) *NetworkService {
	if executor == nil {
		executor = DefaultExecutor()
	}
	return &NetworkService{executor: executor}
}

// GetExternalIP returns the external IP address
func (s *NetworkService) GetExternalIP() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ExternalIPTimeout)
	defer cancel()
	// -4 keeps the lookup to A records. Without it the router resolves
	// AF_UNSPEC and asks for AAAA too - an answer it cannot use, since IPv6 is
	// off - and that query intermittently goes unanswered: glibc sits out its
	// full five seconds before retrying, --connect-timeout 5 fires first, and
	// curl exits 6. Interleaved on an RT-AX86U that was 16 failures in 30
	// tries; with -4, none in 30.
	result, err := s.executor.Exec(ctx, "curl", "-4", "-s", "--connect-timeout", "5", "--max-time", "10", "ifconfig.me")
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		if name, ok := curlErrors[result.ExitCode]; ok {
			return "", fmt.Errorf("curl: %s (exit code %d)", name, result.ExitCode)
		}
		return "", fmt.Errorf("curl failed with exit code %d", result.ExitCode)
	}
	ip := strings.TrimSpace(result.Output)
	// Validate that the result is a valid IP address
	if net.ParseIP(ip) == nil {
		return "", fmt.Errorf("invalid IP address: %s", ip)
	}
	return ip, nil
}
