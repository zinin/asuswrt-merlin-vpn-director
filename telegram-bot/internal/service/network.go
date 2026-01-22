// internal/service/network.go
package service

import "strings"

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
	return strings.TrimSpace(result.Output), nil
}
