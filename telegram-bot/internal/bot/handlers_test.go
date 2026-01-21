package bot

import (
	"testing"

	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/vpnconfig"
)

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

func TestGroupServersByCountry(t *testing.T) {
	tests := []struct {
		name     string
		servers  []vpnconfig.Server
		expected string
	}{
		{
			name:     "empty list",
			servers:  []vpnconfig.Server{},
			expected: "",
		},
		{
			name: "single country",
			servers: []vpnconfig.Server{
				{Name: "Германия, Берлин"},
				{Name: "Германия, Франкфурт"},
			},
			expected: "Германия (2)",
		},
		{
			name: "multiple countries sorted by count",
			servers: []vpnconfig.Server{
				{Name: "США, Нью-Йорк"},
				{Name: "Германия, Берлин"},
				{Name: "США, Майами"},
				{Name: "США, Лос-Анджелес"},
				{Name: "Германия, Франкфурт"},
			},
			expected: "США (3), Германия (2)",
		},
		{
			name: "same count sorted alphabetically",
			servers: []vpnconfig.Server{
				{Name: "Германия, Берлин"},
				{Name: "Австрия, Вена"},
			},
			expected: "Австрия (1), Германия (1)",
		},
		{
			name: "more than 10 countries",
			servers: []vpnconfig.Server{
				{Name: "A, City"}, {Name: "A, City"},
				{Name: "B, City"},
				{Name: "C, City"},
				{Name: "D, City"},
				{Name: "E, City"},
				{Name: "F, City"},
				{Name: "G, City"},
				{Name: "H, City"},
				{Name: "I, City"},
				{Name: "J, City"},
				{Name: "K, City"},
				{Name: "L, City"},
			},
			expected: "A (2), B (1), C (1), D (1), E (1), F (1), G (1), H (1), I (1), J (1), и ещё 2 стран",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := groupServersByCountry(tt.servers)
			if got != tt.expected {
				t.Errorf("groupServersByCountry() = %q, want %q", got, tt.expected)
			}
		})
	}
}
