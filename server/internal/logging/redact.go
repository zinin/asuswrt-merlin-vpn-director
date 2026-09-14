package logging

import (
	"io"
	"regexp"
)

// A Telegram bot token is a numeric bot id, a colon and a secret, and it
// travels in the request path: https://api.telegram.org/bot<id>:<secret>/getMe.
// net/http puts the whole URL into every *url.Error it returns, so a bot that
// cannot reach Telegram logs its own credentials on each retry - six copies in
// /tmp/telegram-bot.log on the author's router, a file that is mode 0644 and is
// shown by the Web UI.
//
// The bot id is public and stays, so a log still says which bot it is talking
// about; only the secret half is replaced. The digit and length floors keep
// ordinary log text out of the match: timestamps, "addr=127.0.0.1:12346" and
// durations all carry colons, but nothing on the right of the colon is twenty
// characters of the token alphabet.
var tokenRe = regexp.MustCompile(`([0-9]{6,}):([A-Za-z0-9_-]{20,})`)

const tokenReplacement = "$1:REDACTED"

// redactSecrets replaces every bot token in p with the bot id and "REDACTED".
func redactSecrets(p []byte) []byte {
	return tokenRe.ReplaceAll(p, []byte(tokenReplacement))
}

// redactingWriter scrubs secrets from everything written through it.
//
// It sits under the log writer rather than in slog's ReplaceAttr so that it
// also covers the standard log package, which NewSlogLogger points at the same
// destination for dependencies - the Telegram client writes full request URLs
// there when its own debug mode is on.
//
// One Write is one log record for both slog and the log package, so a token is
// never split across two calls.
type redactingWriter struct {
	w io.Writer
}

func (rw *redactingWriter) Write(p []byte) (int, error) {
	if _, err := rw.w.Write(redactSecrets(p)); err != nil {
		return 0, err
	}
	// Report what the caller handed over, not what was written: redaction
	// shortens the text, and a smaller count reads as a short write.
	return len(p), nil
}
