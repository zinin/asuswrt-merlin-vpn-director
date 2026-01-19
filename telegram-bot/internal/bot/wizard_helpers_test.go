package bot

import "testing"

func TestGetServerGridColumns(t *testing.T) {
	tests := []struct {
		count    int
		expected int
	}{
		{1, 1},
		{5, 1},
		{6, 2},
		{10, 2},
		{11, 3},
		{57, 3},
		{100, 3},
	}
	for _, tt := range tests {
		got := getServerGridColumns(tt.count)
		if got != tt.expected {
			t.Errorf("getServerGridColumns(%d) = %d, want %d", tt.count, got, tt.expected)
		}
	}
}

func TestTruncateServerName(t *testing.T) {
	tests := []struct {
		name     string
		maxLen   int
		expected string
	}{
		{"Short", 20, "Short"},
		{"This is a very long server name", 15, "This is a ve..."},
		{"Exact15CharLen", 14, "Exact15CharLen"},
		{"Exact", 5, "Exact"},
		{"TooLong", 5, "To..."},
		{"AnyName", 0, ""},
		{"AnyName", -1, ""},
	}
	for _, tt := range tests {
		got := truncateServerName(tt.name, tt.maxLen)
		if got != tt.expected {
			t.Errorf("truncateServerName(%q, %d) = %q, want %q", tt.name, tt.maxLen, got, tt.expected)
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
