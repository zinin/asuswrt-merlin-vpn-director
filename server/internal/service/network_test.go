// internal/service/network_test.go
package service

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/shell"
)

func TestNetworkService_GetExternalIP(t *testing.T) {
	mock := &mockExecutor{
		result: &shell.Result{Output: "1.2.3.4", ExitCode: 0},
	}
	svc := NewNetworkService(mock)

	ip, err := svc.GetExternalIP()
	if err != nil {
		t.Fatalf("GetExternalIP error: %v", err)
	}
	if ip != "1.2.3.4" {
		t.Errorf("expected 1.2.3.4, got %s", ip)
	}
}

func TestNetworkService_GetExternalIP_Error(t *testing.T) {
	mock := &mockExecutor{
		err: errors.New("network error"),
	}
	svc := NewNetworkService(mock)

	_, err := svc.GetExternalIP()
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestNetworkService_GetExternalIP_TrimSpace(t *testing.T) {
	mock := &mockExecutor{
		result: &shell.Result{Output: "  5.6.7.8\n", ExitCode: 0},
	}
	svc := NewNetworkService(mock)

	ip, err := svc.GetExternalIP()
	if err != nil {
		t.Fatalf("GetExternalIP error: %v", err)
	}
	if ip != "5.6.7.8" {
		t.Errorf("expected trimmed IP '5.6.7.8', got %q", ip)
	}
}

func TestNetworkService_GetExternalIP_NonZeroExitCode(t *testing.T) {
	mock := &mockExecutor{
		result: &shell.Result{Output: "", ExitCode: 1},
	}
	svc := NewNetworkService(mock)

	_, err := svc.GetExternalIP()
	if err == nil {
		t.Error("expected error for non-zero exit code, got nil")
	}
}

func TestNetworkService_GetExternalIP_InvalidIPFormat(t *testing.T) {
	mock := &mockExecutor{
		result: &shell.Result{Output: "<html>error page</html>", ExitCode: 0},
	}
	svc := NewNetworkService(mock)

	_, err := svc.GetExternalIP()
	if err == nil {
		t.Error("expected error for invalid IP format, got nil")
	}
}

func TestNetworkService_GetExternalIP_UsesTimeout(t *testing.T) {
	mock := &mockExecutor{result: &shell.Result{Output: "1.2.3.4"}}
	svc := NewNetworkService(mock)
	if _, err := svc.GetExternalIP(); err != nil {
		t.Fatalf("GetExternalIP error: %v", err)
	}
	if len(mock.timeouts) != 1 {
		t.Fatalf("expected one context deadline, got %d", len(mock.timeouts))
	}
	assertTimeout(t, mock.timeouts[0], ExternalIPTimeout)
}

// The router resolves a bare hostname for both A and AAAA. With IPv6 off the
// AAAA answer is unusable, and on this hardware the query is intermittently
// unanswered: glibc sits out its full five seconds before retrying, the
// --connect-timeout fires first and curl exits 6. Measured interleaved on an
// RT-AX86U: 16 failures in 30 tries without -4, none in 30 with it.
func TestNetworkService_GetExternalIP_ResolvesIPv4Only(t *testing.T) {
	mock := &mockExecutor{result: &shell.Result{Output: "1.2.3.4"}}
	svc := NewNetworkService(mock)

	if _, err := svc.GetExternalIP(); err != nil {
		t.Fatalf("GetExternalIP error: %v", err)
	}

	if len(mock.calls) != 1 {
		t.Fatalf("expected one call, got %d", len(mock.calls))
	}
	if !slices.Contains(mock.calls[0], "-4") {
		t.Errorf("curl args = %v, want -4 among them: the dual-family lookup is what fails", mock.calls[0])
	}
}

// A bare "exit code 6" reached the Web UI and told nobody anything; naming the
// failure is what turns the panel into a diagnosis. An unlisted code still
// reports its number rather than nothing.
func TestNetworkService_GetExternalIP_NamesTheCurlFailure(t *testing.T) {
	cases := []struct {
		exit int
		want string
	}{
		{6, "could not resolve host"},
		{7, "could not connect"},
		{28, "timed out"},
		{1, "exit code 1"},
	}
	for _, tc := range cases {
		mock := &mockExecutor{result: &shell.Result{ExitCode: tc.exit}}

		_, err := NewNetworkService(mock).GetExternalIP()

		if err == nil {
			t.Fatalf("exit %d: expected an error, got nil", tc.exit)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("exit %d: error = %q, want it to contain %q", tc.exit, err, tc.want)
		}
	}
}
