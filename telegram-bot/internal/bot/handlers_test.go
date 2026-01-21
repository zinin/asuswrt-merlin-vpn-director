package bot

import (
	"fmt"
	"strings"
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

func TestBuildCodeBlockText(t *testing.T) {
	tests := []struct {
		name            string
		header          string
		content         string
		maxLen          int
		expectTruncated bool
		expectPrefix    string
	}{
		{
			name:            "short content fits",
			header:          "Header:",
			content:         "line1\nline2\nline3",
			maxLen:          4000,
			expectTruncated: false,
			expectPrefix:    "Header:\n```\nline1\nline2\nline3```",
		},
		{
			name:            "long content truncated",
			header:          "H:",
			content:         "aaa\nbbb\nccc\nddd\neee",
			maxLen:          25,
			expectTruncated: true,
			expectPrefix:    "H:\n```\n...\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildCodeBlockText(tt.header, tt.content, tt.maxLen)
			if tt.expectTruncated {
				if !strings.HasPrefix(got, tt.expectPrefix) {
					t.Errorf("buildCodeBlockText() should start with %q, got %q", tt.expectPrefix, got)
				}
				if !strings.Contains(got, "...") {
					t.Errorf("buildCodeBlockText() should contain truncation indicator ...")
				}
			} else {
				if got != tt.expectPrefix {
					t.Errorf("buildCodeBlockText() = %q, want %q", got, tt.expectPrefix)
				}
			}
			if !strings.HasSuffix(got, "```") {
				t.Errorf("buildCodeBlockText() should end with ```, got %q", got)
			}
		})
	}
}

func TestBuildServersPageText(t *testing.T) {
	servers := make([]vpnconfig.Server, 47)
	for i := range servers {
		servers[i] = vpnconfig.Server{
			Name:    fmt.Sprintf("Server-%d, City", i+1),
			Address: fmt.Sprintf("srv%d.example.com", i+1),
			IP:      fmt.Sprintf("1.2.3.%d", i+1),
		}
	}

	tests := []struct {
		name          string
		page          int
		expectHeader  string
		expectFirst   string
		expectLast    string
		expectButtons int // number of navigation buttons
	}{
		{
			name:          "first page",
			page:          0,
			expectHeader:  "🖥 *Servers* \\(47\\), page 1/4:",
			expectFirst:   "1\\. Server\\-1",
			expectLast:    "15\\. Server\\-15",
			expectButtons: 2, // [1/4] [Next →]
		},
		{
			name:          "middle page",
			page:          1,
			expectHeader:  "🖥 *Servers* \\(47\\), page 2/4:",
			expectFirst:   "16\\. Server\\-16",
			expectLast:    "30\\. Server\\-30",
			expectButtons: 3, // [← Prev] [2/4] [Next →]
		},
		{
			name:          "last page",
			page:          3,
			expectHeader:  "🖥 *Servers* \\(47\\), page 4/4:",
			expectFirst:   "46\\. Server\\-46",
			expectLast:    "47\\. Server\\-47",
			expectButtons: 2, // [← Prev] [4/4]
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, keyboard := buildServersPage(servers, tt.page)

			if !strings.Contains(text, tt.expectHeader) {
				t.Errorf("page text should contain %q, got:\n%s", tt.expectHeader, text)
			}
			if !strings.Contains(text, tt.expectFirst) {
				t.Errorf("page text should contain first server %q", tt.expectFirst)
			}
			if !strings.Contains(text, tt.expectLast) {
				t.Errorf("page text should contain last server %q", tt.expectLast)
			}

			if len(keyboard.InlineKeyboard) != 1 {
				t.Errorf("expected 1 keyboard row, got %d", len(keyboard.InlineKeyboard))
			}
			if len(keyboard.InlineKeyboard[0]) != tt.expectButtons {
				t.Errorf("expected %d buttons, got %d", tt.expectButtons, len(keyboard.InlineKeyboard[0]))
			}
		})
	}
}
