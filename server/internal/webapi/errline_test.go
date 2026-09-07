package webapi

import (
	"errors"
	"strings"
	"testing"
)

func TestLastErrorLine(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"single line", errors.New("apply failed (exit 1): boom"), "apply failed (exit 1): boom"},
		{"last non-empty line wins", errors.New("apply failed (exit 1): [INFO] step\n[ERROR] Invalid country code 'xx'\n\n"), "[ERROR] Invalid country code 'xx'"},
		{"trims spaces", errors.New("x\n   padded   \n"), "padded"},
		{"caps at 200 runes and marks the cut", errors.New(strings.Repeat("я", 250)), strings.Repeat("я", 200) + "…"},
		{"exactly 200 runes is not marked", errors.New(strings.Repeat("я", 200)), strings.Repeat("я", 200)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lastErrorLine(tt.err); got != tt.want {
				t.Errorf("lastErrorLine() = %q, want %q", got, tt.want)
			}
		})
	}
}
