package wizard

import "testing"

func TestIsValidIPOrCIDR(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"1.2.3.4", true},
		{"10.0.0.0/8", true},
		{"192.168.1.0/24", true},
		{"not-an-ip", false},
		{"256.1.1.1", false},
		{"", false},
		{"::1", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := IsValidIPOrCIDR(tt.input); got != tt.valid {
				t.Errorf("IsValidIPOrCIDR(%q) = %v, want %v", tt.input, got, tt.valid)
			}
		})
	}
}
