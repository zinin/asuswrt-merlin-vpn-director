package bot

import "testing"

func TestEscapeMarkdownV2(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "dots and dashes",
			input:    "192.168.1.100",
			expected: "192\\.168\\.1\\.100",
		},
		{
			name:     "parentheses",
			input:    "Step (1/4)",
			expected: "Step \\(1/4\\)",
		},
		{
			name:     "error message with special chars",
			input:    "Error: file_not_found (code: -1)",
			expected: "Error: file\\_not\\_found \\(code: \\-1\\)",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "plain text",
			input:    "Hello World",
			expected: "Hello World",
		},
		{
			name:     "all special chars",
			input:    "_*[]()~`>#+-=|{}.!",
			expected: "\\_\\*\\[\\]\\(\\)\\~\\`\\>\\#\\+\\-\\=\\|\\{\\}\\.\\!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeMarkdownV2(tt.input)
			if got != tt.expected {
				t.Errorf("escapeMarkdownV2(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
