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
	result, err := s.executor.Exec(ctx, "curl", "-s", "--connect-timeout", "5", "--max-time", "10", "ifconfig.me")
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("curl failed with exit code %d", result.ExitCode)
	}
	ip := strings.TrimSpace(result.Output)
	// Validate that the result is a valid IP address
	if net.ParseIP(ip) == nil {
		return "", fmt.Errorf("invalid IP address: %s", ip)
	}
	return ip, nil
}
