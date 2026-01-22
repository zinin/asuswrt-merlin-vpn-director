// internal/handler/import.go
package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/telegram"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/vless"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/vpnconfig"
)

// ImportHandler handles /import command
type ImportHandler struct {
	deps        *Deps
	httpClient  *http.Client
	maxBodySize int64
}

// NewImportHandler creates a new ImportHandler with default timeout and size limits
func NewImportHandler(deps *Deps) *ImportHandler {
	return &ImportHandler{
		deps:        deps,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		maxBodySize: 1 << 20, // 1MB
	}
}

// HandleImport handles /import command - downloads and imports VLESS subscription
func (h *ImportHandler) HandleImport(msg *tgbotapi.Message) {
	args := msg.CommandArguments()
	if args == "" {
		h.deps.Sender.Send(msg.Chat.ID, "Usage: `/import <url>`")
		return
	}

	// Validate URL scheme
	parsedURL, err := url.Parse(args)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		h.deps.Sender.Send(msg.Chat.ID, "Invalid URL\\. Use http:// or https://")
		return
	}

	h.deps.Sender.Send(msg.Chat.ID, "Loading server list\\.\\.\\.")

	// Download subscription
	resp, err := h.httpClient.Get(args)
	if err != nil {
		h.deps.Sender.Send(msg.Chat.ID, telegram.EscapeMarkdownV2(fmt.Sprintf("Download error: %v", err)))
		return
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		h.deps.Sender.Send(msg.Chat.ID, fmt.Sprintf("Error: HTTP %d", resp.StatusCode))
		return
	}

	// Limit body size
	body, err := io.ReadAll(io.LimitReader(resp.Body, h.maxBodySize))
	if err != nil {
		h.deps.Sender.Send(msg.Chat.ID, telegram.EscapeMarkdownV2(fmt.Sprintf("Read error: %v", err)))
		return
	}

	// Decode VLESS subscription
	servers, parseErrors := vless.DecodeSubscription(string(body))
	if len(servers) == 0 {
		var sb strings.Builder
		sb.WriteString("No VLESS servers found")
		if len(parseErrors) > 0 {
			sb.WriteString("\nErrors:\n")
			for _, e := range parseErrors {
				sb.WriteString(fmt.Sprintf("- %s\n", e))
			}
		}
		h.deps.Sender.Send(msg.Chat.ID, telegram.EscapeMarkdownV2(sb.String()))
		return
	}

	// Resolve IPs for each server
	var resolved []vpnconfig.Server
	var resolveErrors int
	totalParsed := len(servers)

	for _, s := range servers {
		if err := s.ResolveIP(); err != nil {
			resolveErrors++
			continue
		}
		resolved = append(resolved, vpnconfig.Server{
			Address: s.Address,
			Port:    s.Port,
			UUID:    s.UUID,
			Name:    s.Name,
			IP:      s.IP,
		})
	}

	if len(resolved) == 0 {
		h.deps.Sender.Send(msg.Chat.ID, "Could not resolve IP for any server")
		return
	}

	// Get data directory and ensure it exists
	dataDir := h.deps.Config.DataDirOrDefault()
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		h.deps.Sender.Send(msg.Chat.ID, telegram.EscapeMarkdownV2(fmt.Sprintf("Directory creation error: %v", err)))
		return
	}

	// Save servers
	if err := h.deps.Config.SaveServers(resolved); err != nil {
		h.deps.Sender.Send(msg.Chat.ID, telegram.EscapeMarkdownV2(fmt.Sprintf("Save error: %v", err)))
		return
	}

	// Build response with grouped stats
	var sb strings.Builder
	if resolveErrors > 0 || len(parseErrors) > 0 {
		sb.WriteString(fmt.Sprintf("Imported %d of %d servers:\n", len(resolved), totalParsed))
	} else {
		sb.WriteString(fmt.Sprintf("Imported %d servers:\n", len(resolved)))
	}

	groupedStr := groupServersByCountry(resolved)
	sb.WriteString(telegram.EscapeMarkdownV2(groupedStr))

	if resolveErrors > 0 || len(parseErrors) > 0 {
		sb.WriteString("\n\n")
		if resolveErrors > 0 {
			sb.WriteString(fmt.Sprintf("%d DNS errors", resolveErrors))
		}
		if len(parseErrors) > 0 {
			if resolveErrors > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("%d parse errors", len(parseErrors)))
		}
	}

	h.deps.Sender.Send(msg.Chat.ID, sb.String())
}
