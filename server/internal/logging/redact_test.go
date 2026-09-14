package logging

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The shape Telegram hands out: a numeric bot id, a colon, then the secret.
const (
	testBotID  = "7950377903"
	testSecret = "AAGdBcn23W_XHmbQPnwc9Xm4AW9HRLMfKn0"
	testToken  = testBotID + ":" + testSecret
)

func TestRedactSecrets_HidesATokenInsideAnAPIURL(t *testing.T) {
	// Exactly what leaked: net/http puts the request URL in the error, and
	// Telegram carries the token in the path.
	in := fmt.Sprintf(`Post "https://api.telegram.org/bot%s/getMe": net/http: TLS handshake timeout`, testToken)

	got := string(redactSecrets([]byte(in)))

	if strings.Contains(got, testSecret) {
		t.Fatalf("secret survived redaction: %q", got)
	}
	if !strings.Contains(got, testBotID) {
		t.Errorf("bot id should survive so a log still says which bot: %q", got)
	}
	if !strings.Contains(got, "getMe") {
		t.Errorf("the rest of the URL should survive: %q", got)
	}
}

func TestRedactSecrets_HidesABareToken(t *testing.T) {
	got := string(redactSecrets([]byte("token=" + testToken + " loaded")))

	if strings.Contains(got, testSecret) {
		t.Fatalf("secret survived redaction: %q", got)
	}
	if !strings.Contains(got, "loaded") {
		t.Errorf("surrounding text should survive: %q", got)
	}
}

func TestRedactSecrets_LeavesOrdinaryLinesAlone(t *testing.T) {
	// Timestamps, durations and addresses all carry colons; none of them may
	// be mangled.
	in := `time=2026-09-14T21:48:19.205+03:00 level=WARN msg="dial" addr=127.0.0.1:12346 duration=239.462µs`

	if got := string(redactSecrets([]byte(in))); got != in {
		t.Errorf("ordinary line was altered:\n before: %q\n after:  %q", in, got)
	}
}

func TestRedactingWriter_ReportsTheWholeInputAsWritten(t *testing.T) {
	// An io.Writer that reports fewer bytes than it was given makes callers
	// treat a successful write as a short write. Redaction shortens the text,
	// so the count has to come from the input.
	var buf bytes.Buffer
	w := &redactingWriter{w: &buf}
	in := []byte("token=" + testToken + "\n")

	n, err := w.Write(in)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(in) {
		t.Errorf("Write reported %d of %d bytes", n, len(in))
	}
	if strings.Contains(buf.String(), testSecret) {
		t.Errorf("secret reached the underlying writer: %q", buf.String())
	}
}

func TestNewSlogLogger_KeepsATokenOutOfTheLogFile(t *testing.T) {
	// The end the defect was found at: a *url.Error logged as an attribute.
	path := filepath.Join(t.TempDir(), "bot.log")
	logger, lg, err := NewSlogLogger(path)
	if err != nil {
		t.Fatalf("NewSlogLogger: %v", err)
	}
	defer lg.Close()

	apiErr := &url.Error{
		Op:  "Post",
		URL: "https://api.telegram.org/bot" + testToken + "/getMe",
		Err: errors.New("net/http: TLS handshake timeout"),
	}
	logger.Warn("Failed to connect to Telegram API, retrying...", "error", apiErr)

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(written), testSecret) {
		t.Fatalf("token written to the log file:\n%s", written)
	}
	if !strings.Contains(string(written), "TLS handshake timeout") {
		t.Errorf("the useful half of the error was lost:\n%s", written)
	}
}
