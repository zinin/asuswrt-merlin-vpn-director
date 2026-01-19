package bot

import "testing"

func TestGetServerGridColumns(t *testing.T) {
	tests := []struct {
		count    int
		expected int
	}{
		{1, 1},
		{5, 1},
		{10, 1},
		{11, 2},
		{57, 2},
		{100, 2},
	}
	for _, tt := range tests {
		got := getServerGridColumns(tt.count)
		if got != tt.expected {
			t.Errorf("getServerGridColumns(%d) = %d, want %d", tt.count, got, tt.expected)
		}
	}
}

func TestFormatExclusionButton(t *testing.T) {
	tests := []struct {
		code     string
		selected bool
		expected string
	}{
		{"ru", true, "✅ ru Russia"},
		{"ru", false, "🔲 ru Russia"},
		{"de", true, "✅ de Germany"},
		{"unknown", false, "🔲 unknown unknown"},
	}
	for _, tt := range tests {
		got := formatExclusionButton(tt.code, tt.selected)
		if got != tt.expected {
			t.Errorf("formatExclusionButton(%q, %v) = %q, want %q", tt.code, tt.selected, got, tt.expected)
		}
	}
}
