// Package telegram provides Telegram-specific utilities
package telegram

import (
	"fmt"
	"strings"
)

// MaxMessageLength is the maximum length for a Telegram message
const MaxMessageLength = 4000

// EscapeMarkdownV2 escapes special characters for Telegram MarkdownV2
func EscapeMarkdownV2(text string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"~", "\\~",
		"`", "\\`",
		">", "\\>",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		"=", "\\=",
		"|", "\\|",
		"{", "\\{",
		"}", "\\}",
		".", "\\.",
		"!", "\\!",
	)
	return replacer.Replace(text)
}

// CodeBlock wraps content in Telegram code block
func CodeBlock(content string) string {
	return fmt.Sprintf("```\n%s```", content)
}

// BuildCodeBlockMessage builds a code block message with optional truncation
func BuildCodeBlockMessage(header, content string, maxLen int) string {
	maxContentLen := maxLen - len(header) - 10

	if len(content) > maxContentLen {
		content = content[len(content)-maxContentLen:]
		if idx := strings.Index(content, "\n"); idx != -1 {
			content = content[idx+1:]
		}
		content = "...\n" + content
	}

	return fmt.Sprintf("%s\n```\n%s```", header, content)
}
