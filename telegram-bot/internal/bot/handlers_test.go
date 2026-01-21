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

func TestExtractCountry(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "country and city",
			input:    "Чехия, Прага",
			expected: "Чехия",
		},
		{
			name:     "country and extra",
			input:    "Германия, Extra",
			expected: "Германия",
		},
		{
			name:     "english format",
			input:    "Germany, Berlin",
			expected: "Germany",
		},
		{
			name:     "with leading/trailing spaces",
			input:    "  Россия , Москва  ",
			expected: "Россия",
		},
		{
			name:     "no comma",
			input:    "Unknown Server",
			expected: "Unknown Server",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "Other",
		},
		{
			name:     "only spaces",
			input:    "   ",
			expected: "Other",
		},
		{
			name:     "comma at start",
			input:    ", City",
			expected: "Other",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractCountry(tt.input)
			if got != tt.expected {
				t.Errorf("extractCountry(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
