// internal/service/network.go
package service

import (
	"fmt"
	"net"
	"strings"
)

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
	result, err := s.executor.Exec("curl", "-s", "--connect-timeout", "5", "--max-time", "10", "ifconfig.me")
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
