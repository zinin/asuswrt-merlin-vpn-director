package webapi

import "strings"

// maxErrorLineRunes caps the shell output line echoed in API error messages.
const maxErrorLineRunes = 200

// lastErrorLine returns the last non-empty line of err's message, trimmed
// and capped at maxErrorLineRunes. Shell failures carry the whole script
// output; the last line is the ERROR line the user needs to see.
func lastErrorLine(err error) string {
	if err == nil {
		return ""
	}
	lines := strings.Split(err.Error(), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if r := []rune(line); len(r) > maxErrorLineRunes {
			line = string(r[:maxErrorLineRunes])
		}
		return line
	}
	return ""
}
