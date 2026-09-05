package vpnconfig

import (
	"errors"
	"testing"
)

func TestNormalizeClientAddr(t *testing.T) {
	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{"192.168.50.10", "192.168.50.10", true},
		{"  192.168.50.10  ", "192.168.50.10", true},
		{"192.168.50.10/32", "192.168.50.10", true},
		{"192.168.50.0/24", "192.168.50.0/24", true},
		{"10.0.0.0/8", "10.0.0.0/8", true},
		{"192.168.50.10/24", "192.168.50.0/24", true},
		{"1.2.3.4/032", "1.2.3.4", true},
		{"", "", false},
		{"not-an-ip", "", false},
		{"256.1.1.1", "", false},
		{"1.2.3.4/33", "", false},
		{"1.2.3.4/", "", false},
		{"::1", "", false},
		{"::1/128", "", false},
		{"2001:db8::/32", "", false},
		{"::ffff:1.2.3.4", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := NormalizeClientAddr(tt.input)
			if !tt.ok {
				if err == nil {
					t.Fatalf("NormalizeClientAddr(%q) = %q, want error", tt.input, got)
				}
				if !errors.Is(err, ErrInvalidClientAddr) {
					t.Errorf("error = %v, want ErrInvalidClientAddr", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeClientAddr(%q) error = %v, want nil", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("NormalizeClientAddr(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestErrInvalidClientAddrText(t *testing.T) {
	// Six later tasks return this verbatim as the HTTP 400 body.
	if got := ErrInvalidClientAddr.Error(); got != "invalid IPv4 address or CIDR" {
		t.Errorf("ErrInvalidClientAddr.Error() = %q, want %q", got, "invalid IPv4 address or CIDR")
	}
}
