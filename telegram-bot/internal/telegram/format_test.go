package telegram

import (
	"strings"
	"testing"
)

func TestEscapeMarkdownV2(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"dots and dashes", "192.168.1.100", "192\\.168\\.1\\.100"},
		{"parentheses", "Step (1/4)", "Step \\(1/4\\)"},
		{"empty string", "", ""},
		{"all special chars", "_*[]()~`>#+-=|{}.!", "\\_\\*\\[\\]\\(\\)\\~\\`\\>\\#\\+\\-\\=\\|\\{\\}\\.\\!"},
		{"backslash", "path\\to\\file", "path\\\\to\\\\file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EscapeMarkdownV2(tt.input)
			if got != tt.expected {
				t.Errorf("EscapeMarkdownV2(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestCodeBlock(t *testing.T) {
	got := CodeBlock("hello\nworld")
	expected := "```\nhello\nworld```"
	if got != expected {
		t.Errorf("CodeBlock() = %q, want %q", got, expected)
	}
}

func TestBuildCodeBlockMessage(t *testing.T) {
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
			content:         "line1\nline2",
			maxLen:          4000,
			expectTruncated: false,
			expectPrefix:    "Header:\n```\nline1\nline2```",
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
			got := BuildCodeBlockMessage(tt.header, tt.content, tt.maxLen)
			if tt.expectTruncated {
				if !strings.HasPrefix(got, tt.expectPrefix) {
					t.Errorf("should start with %q, got %q", tt.expectPrefix, got)
				}
			} else {
				if got != tt.expectPrefix {
					t.Errorf("got %q, want %q", got, tt.expectPrefix)
				}
			}
			if !strings.HasSuffix(got, "```") {
				t.Errorf("should end with ```, got %q", got)
			}
		})
	}
}
