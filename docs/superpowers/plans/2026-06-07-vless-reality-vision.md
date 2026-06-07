# VLESS REALITY/Vision Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Parse and persist per-server VLESS stream parameters (REALITY + TLS) end-to-end so generated Xray configs connect to REALITY/XTLS-Vision servers, fixing the Telegram bot (socks5) and LAN TPROXY.

**Architecture:** Extend the `Server` schema (Go `vless` + `vpnconfig`, shell `servers.json`) with `security/network/flow/sni/fingerprint/public_key/short_id/alpn`. Both parsers read URI query params; config generation replaces the `outbounds` array wholesale built from those params — `encoding/json` in Go (`xray.go`), a sourceable jq helper (`lib/xrayconf.sh`) in shell. The template becomes valid JSON with empty `outbounds`.

**Tech Stack:** Go (`encoding/json`, `net/url`), Bash + `jq`, bats (bats-support/bats-assert), `go test`.

**Spec:** `docs/superpowers/specs/2026-06-07-vless-reality-design.md`

**Conventions:**
- Go tests: `cd server && go test ./...` (delegate to build-runner agent).
- Bats tests: `bats router/test/...` (delegate to build-runner agent).
- Legacy = empty `security`: output must be **semantically** identical to the old TLS config (`security:"tls"`, `serverName`=address, `alpn:["h2"]`, no `flow`). JSON key order may differ; Xray is order-insensitive — tests assert parsed structure, never bytes.

---

### Task 1: Make `config.json.template` valid JSON

**Files:**
- Modify: `router/opt/etc/xray/config.json.template`
- Modify: `server/testdata/dev/xray.template.json` (dev-mode template; carries the SAME `{{...}}` placeholders — `paths.go:34` → `bot.go:96` / `webui/main.go:106`. After Task 4 rewrites Go `GenerateConfig` to `json.Unmarshal`, an invalid-JSON dev template breaks dev-mode generation. Must also become valid JSON with `outbounds: []`.)
- Test: `router/test/unit/xray_template.bats` (create)

- [ ] **Step 1: Write the failing test**

Create `router/test/unit/xray_template.bats`:

```bash
#!/usr/bin/env bats
load '../test_helper.bash'

@test "config.json.template is valid JSON" {
    run jq empty "$PROJECT_ROOT/opt/etc/xray/config.json.template"
    [ "$status" -eq 0 ]
}

@test "config.json.template has empty outbounds (filled by generator)" {
    run jq -e '.outbounds == []' "$PROJECT_ROOT/opt/etc/xray/config.json.template"
    [ "$status" -eq 0 ]
}

@test "config.json.template keeps both inbounds (tproxy-in, socks-in)" {
    run jq -e '.inbounds | length == 2' "$PROJECT_ROOT/opt/etc/xray/config.json.template"
    [ "$status" -eq 0 ]
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bats router/test/unit/xray_template.bats`
Expected: FAIL — current template has `{{XRAY_SERVER_PORT}}` (invalid JSON) and a non-empty outbound.

- [ ] **Step 3: Edit the template**

In `router/opt/etc/xray/config.json.template`, replace the entire `"outbounds": [ ... ]` array (the `proxy-out` object with all `{{...}}` placeholders) with an empty array. The `inbounds` and `routing` blocks stay unchanged. Result:

```json
{
  "inbounds": [
    {
      "port": 12345,
      "listen": "0.0.0.0",
      "protocol": "dokodemo-door",
      "settings": { "network": "tcp,udp", "followRedirect": true },
      "sniffing": { "enabled": true, "destOverride": ["http", "tls", "quic"], "routeOnly": false },
      "streamSettings": { "sockopt": { "tproxy": "tproxy" } },
      "tag": "tproxy-in"
    },
    {
      "port": 12346,
      "listen": "127.0.0.1",
      "protocol": "socks",
      "settings": { "udp": true },
      "tag": "socks-in"
    }
  ],
  "outbounds": [],
  "routing": {
    "domainStrategy": "AsIs",
    "rules": [
      { "type": "field", "inboundTag": ["tproxy-in", "socks-in"], "outboundTag": "proxy-out" }
    ]
  }
}
```

- [ ] **Step 3b: Make the dev-mode template valid JSON too**

`server/testdata/dev/xray.template.json` carries the same `{{XRAY_SERVER_ADDRESS}}` / `{{XRAY_SERVER_PORT}}` / `{{XRAY_USER_UUID}}` placeholders and is loaded in dev mode (`paths.go:34` → `bot.go:96`, `webui/main.go:106`). Since Task 4 switches Go generation to `json.Unmarshal`, an invalid-JSON dev template would make dev-mode `GenerateConfig` fail at parse time. Replace its `"outbounds": [ ... ]` (the placeholder `proxy-out` object) with `"outbounds": []`, leaving `inbounds`/`routing` intact, so it parses as valid JSON. (Go `xray_test.go` uses its own in-memory `testTemplate`, so this fix is for the dev runtime, not the unit tests.)

- [ ] **Step 4: Run test to verify it passes**

Run: `bats router/test/unit/xray_template.bats`
Expected: PASS (all three tests).

- [ ] **Step 5: Commit**

```bash
git add router/opt/etc/xray/config.json.template server/testdata/dev/xray.template.json router/test/unit/xray_template.bats
git commit -m "feat(xray): make config templates valid JSON with empty outbounds"
```

---

### Task 2: Go — parse query params in `vless.ParseURI`

**Files:**
- Modify: `server/internal/vless/parser.go:32-38` (struct), `:40-114` (ParseURI)
- Test: `server/internal/vless/parser_test.go`

- [ ] **Step 1: Write the failing tests**

In `server/internal/vless/parser_test.go`, MODIFY `TestParseURI_ComplexQueryParams` to assert the new fields, and ADD a reality test:

```go
func TestParseURI_ComplexQueryParams(t *testing.T) {
	uri := "vless://uuid@server.com:443?encryption=none&security=tls&sni=server.com&fp=chrome#Name"

	server, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if server.Address != "server.com" {
		t.Errorf("expected Address 'server.com', got '%s'", server.Address)
	}
	if server.Port != 443 {
		t.Errorf("expected Port 443, got %d", server.Port)
	}
	if server.Security != "tls" {
		t.Errorf("expected Security 'tls', got '%s'", server.Security)
	}
	if server.SNI != "server.com" {
		t.Errorf("expected SNI 'server.com', got '%s'", server.SNI)
	}
	if server.Fingerprint != "chrome" {
		t.Errorf("expected Fingerprint 'chrome', got '%s'", server.Fingerprint)
	}
}

func TestParseURI_Reality(t *testing.T) {
	// Real subscription format: headerType=none present, pbk/sid at the end, type after headerType
	// (guards against `type` parsing accidentally matching `headerType`).
	uri := "vless://9ca8@162.249.126.77:443?security=reality&encryption=none&fp=firefox&headerType=none&type=tcp&flow=xtls-rprx-vision&sni=cdn3-87.yahoo.com&pbk=PBKEY&sid=55e6d9bd269aac46#NL"

	s, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Security != "reality" {
		t.Errorf("Security = %q, want reality", s.Security)
	}
	if s.Flow != "xtls-rprx-vision" {
		t.Errorf("Flow = %q, want xtls-rprx-vision", s.Flow)
	}
	if s.Network != "tcp" {
		t.Errorf("Network = %q, want tcp", s.Network)
	}
	if s.SNI != "cdn3-87.yahoo.com" {
		t.Errorf("SNI = %q, want cdn3-87.yahoo.com", s.SNI)
	}
	if s.Fingerprint != "firefox" {
		t.Errorf("Fingerprint = %q, want firefox", s.Fingerprint)
	}
	if s.PublicKey != "PBKEY" {
		t.Errorf("PublicKey = %q, want PBKEY", s.PublicKey)
	}
	if s.ShortID != "55e6d9bd269aac46" {
		t.Errorf("ShortID = %q, want 55e6d9bd269aac46", s.ShortID)
	}
}

func TestParseURI_NoParams(t *testing.T) {
	s, err := ParseURI("vless://uuid@host:443#X")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Security != "" || s.Flow != "" || s.SNI != "" {
		t.Errorf("expected empty stream params, got security=%q flow=%q sni=%q", s.Security, s.Flow, s.SNI)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd server && go test ./internal/vless/ -run 'TestParseURI_ComplexQueryParams|TestParseURI_Reality|TestParseURI_NoParams' -v`
Expected: COMPILE FAIL — `server.Security` etc. undefined.

- [ ] **Step 3: Add fields to the `Server` struct**

Replace the struct at `server/internal/vless/parser.go:32-38`:

```go
type Server struct {
	Address     string   `json:"address"`
	Port        int      `json:"port"`
	UUID        string   `json:"uuid"`
	Name        string   `json:"name"`
	IPs         []string `json:"ips"`
	Security    string   `json:"security,omitempty"`
	Network     string   `json:"network,omitempty"`
	Flow        string   `json:"flow,omitempty"`
	SNI         string   `json:"sni,omitempty"`
	Fingerprint string   `json:"fingerprint,omitempty"`
	PublicKey   string   `json:"public_key,omitempty"`
	ShortID     string   `json:"short_id,omitempty"`
	ALPN        []string `json:"alpn,omitempty"`
}
```

- [ ] **Step 4: Parse the query in `ParseURI`**

In `server/internal/vless/parser.go`, replace the query-stripping block (currently `// Remove query params` at lines 55-58) with parsing:

```go
	// Extract and parse query params
	var params url.Values
	if idx := strings.Index(rest, "?"); idx != -1 {
		params, _ = url.ParseQuery(rest[idx+1:])
		rest = rest[:idx]
	}
```

Then replace the final `return &Server{...}` (lines 108-114) with:

```go
	s := &Server{
		Address: address,
		Port:    port,
		UUID:    uuid,
		Name:    name,
	}
	if params != nil {
		s.Security = params.Get("security")
		s.Network = params.Get("type")
		s.Flow = params.Get("flow")
		s.SNI = params.Get("sni")
		s.Fingerprint = params.Get("fp")
		s.PublicKey = params.Get("pbk")
		s.ShortID = params.Get("sid")
		if alpn := params.Get("alpn"); alpn != "" {
			s.ALPN = strings.Split(alpn, ",")
		}
	}
	return s, nil
```

(`net/url` and `strings` are already imported.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd server && go test ./internal/vless/ -v`
Expected: PASS (all parser tests).

- [ ] **Step 6: Commit**

```bash
git add server/internal/vless/parser.go server/internal/vless/parser_test.go
git commit -m "feat(vless): parse REALITY/TLS stream params from URI"
```

---

### Task 3: Go — add fields to `vpnconfig.Server` + shared converter

**Files:**
- Modify: `server/internal/vpnconfig/vpnconfig.go:9-15`
- Modify: `server/internal/vless/parser.go` (add `ToVPNConfig` method)
- Modify: `server/internal/handler/import.go:98-104`, `server/internal/webapi/handler_servers.go:169-175`
- Test: `server/internal/vless/parser_test.go`

- [ ] **Step 1: Write the failing test**

Add to `server/internal/vless/parser_test.go` (add `vpnconfig` to imports — `"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"`):

```go
func TestToVPNConfig_CarriesStreamParams(t *testing.T) {
	s := &Server{
		Address: "1.2.3.4", Port: 443, UUID: "u", Name: "n", IPs: []string{"1.2.3.4"},
		Security: "reality", Network: "tcp", Flow: "xtls-rprx-vision",
		SNI: "cdn.example.com", Fingerprint: "firefox", PublicKey: "PBK", ShortID: "sid",
	}
	var c vpnconfig.Server = s.ToVPNConfig()
	if c.Security != "reality" || c.Flow != "xtls-rprx-vision" || c.PublicKey != "PBK" || c.ShortID != "sid" || c.SNI != "cdn.example.com" || c.Fingerprint != "firefox" {
		t.Errorf("ToVPNConfig dropped stream params: %+v", c)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd server && go test ./internal/vless/ -run TestToVPNConfig_CarriesStreamParams`
Expected: COMPILE FAIL — `vpnconfig.Server` has no `Security`; `ToVPNConfig` undefined.

- [ ] **Step 3: Add fields to `vpnconfig.Server`**

Replace `server/internal/vpnconfig/vpnconfig.go:9-15`:

```go
type Server struct {
	Address     string   `json:"address"`
	Port        int      `json:"port"`
	UUID        string   `json:"uuid"`
	Name        string   `json:"name"`
	IPs         []string `json:"ips"`
	Security    string   `json:"security,omitempty"`
	Network     string   `json:"network,omitempty"`
	Flow        string   `json:"flow,omitempty"`
	SNI         string   `json:"sni,omitempty"`
	Fingerprint string   `json:"fingerprint,omitempty"`
	PublicKey   string   `json:"public_key,omitempty"`
	ShortID     string   `json:"short_id,omitempty"`
	ALPN        []string `json:"alpn,omitempty"`
}
```

- [ ] **Step 4: Add `ToVPNConfig` to `vless.Server`**

Add `"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"` to imports in `server/internal/vless/parser.go`, then add:

```go
// ToVPNConfig converts a parsed vless.Server into a vpnconfig.Server,
// carrying all stream parameters. Call ResolveIPs first to populate IPs.
func (s *Server) ToVPNConfig() vpnconfig.Server {
	return vpnconfig.Server{
		Address:     s.Address,
		Port:        s.Port,
		UUID:        s.UUID,
		Name:        s.Name,
		IPs:         s.IPs,
		Security:    s.Security,
		Network:     s.Network,
		Flow:        s.Flow,
		SNI:         s.SNI,
		Fingerprint: s.Fingerprint,
		PublicKey:   s.PublicKey,
		ShortID:     s.ShortID,
		ALPN:        s.ALPN,
	}
}
```

- [ ] **Step 5: Update both conversion sites**

In `server/internal/handler/import.go`, replace the literal at lines 98-104 with:

```go
		resolved = append(resolved, s.ToVPNConfig())
```

In `server/internal/webapi/handler_servers.go`, replace the literal at lines 169-175 with:

```go
			resolved = append(resolved, s.ToVPNConfig())
```

- [ ] **Step 6: Run tests + build**

Run: `cd server && go build ./... && go test ./internal/vless/ ./internal/handler/ ./internal/webapi/`
Expected: PASS (build succeeds, tests pass).

- [ ] **Step 7: Commit**

```bash
git add server/internal/vpnconfig/vpnconfig.go server/internal/vless/parser.go server/internal/vless/parser_test.go server/internal/handler/import.go server/internal/webapi/handler_servers.go
git commit -m "feat(vpnconfig): carry stream params from vless to servers.json"
```

---

### Task 4: Go — generate REALITY/TLS outbound in `xray.go`

**Files:**
- Modify: `server/internal/service/xray.go` (full rewrite)
- Test: `server/internal/service/xray_test.go` (full rewrite)

- [ ] **Step 1: Write the failing tests**

Replace `server/internal/service/xray_test.go` entirely:

```go
// internal/service/xray_test.go
package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

const testTemplate = `{"inbounds":[],"outbounds":[],"routing":{}}`

func generate(t *testing.T, server vpnconfig.Server) map[string]interface{} {
	t.Helper()
	tmpDir := t.TempDir()
	templatePath := filepath.Join(tmpDir, "config.json.template")
	outputPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(templatePath, []byte(testTemplate), 0644); err != nil {
		t.Fatal(err)
	}
	if err := NewXrayService(templatePath, outputPath).GenerateConfig(server); err != nil {
		t.Fatalf("GenerateConfig error: %v", err)
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(content, &cfg); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	return cfg
}

func outbound0(t *testing.T, cfg map[string]interface{}) map[string]interface{} {
	t.Helper()
	obs, ok := cfg["outbounds"].([]interface{})
	if !ok || len(obs) != 1 {
		t.Fatalf("expected 1 outbound, got %v", cfg["outbounds"])
	}
	return obs[0].(map[string]interface{})
}

func TestGenerateConfig_Reality(t *testing.T) {
	cfg := generate(t, vpnconfig.Server{
		Address: "162.249.126.77", Port: 443, UUID: "abc-123",
		Security: "reality", Network: "tcp", Flow: "xtls-rprx-vision",
		SNI: "cdn3-87.yahoo.com", Fingerprint: "firefox", PublicKey: "PBKEY", ShortID: "55e6",
	})
	ob := outbound0(t, cfg)
	ss := ob["streamSettings"].(map[string]interface{})
	if ss["security"] != "reality" {
		t.Fatalf("security = %v, want reality", ss["security"])
	}
	rs := ss["realitySettings"].(map[string]interface{})
	if rs["publicKey"] != "PBKEY" || rs["shortId"] != "55e6" || rs["serverName"] != "cdn3-87.yahoo.com" || rs["fingerprint"] != "firefox" {
		t.Errorf("realitySettings wrong: %v", rs)
	}
	if ss["network"] != "tcp" {
		t.Errorf("network = %v, want tcp", ss["network"])
	}
	if _, ok := ss["tlsSettings"]; ok {
		t.Errorf("reality outbound must not contain tlsSettings")
	}
	u0 := ob["settings"].(map[string]interface{})["vnext"].([]interface{})[0].(map[string]interface{})["users"].([]interface{})[0].(map[string]interface{})
	if u0["flow"] != "xtls-rprx-vision" {
		t.Errorf("flow = %v, want xtls-rprx-vision", u0["flow"])
	}
}

func TestGenerateConfig_TLS(t *testing.T) {
	cfg := generate(t, vpnconfig.Server{
		Address: "1.2.3.4", Port: 443, UUID: "u", Security: "tls", SNI: "host.example.com", Fingerprint: "chrome",
	})
	ss := outbound0(t, cfg)["streamSettings"].(map[string]interface{})
	if ss["security"] != "tls" {
		t.Fatalf("security = %v, want tls", ss["security"])
	}
	tls := ss["tlsSettings"].(map[string]interface{})
	if tls["serverName"] != "host.example.com" || tls["fingerprint"] != "chrome" {
		t.Errorf("tlsSettings wrong: %v", tls)
	}
	if _, ok := ss["realitySettings"]; ok {
		t.Errorf("tls outbound must not contain realitySettings")
	}
}

func TestGenerateConfig_Legacy(t *testing.T) {
	cfg := generate(t, vpnconfig.Server{Address: "example.com", Port: 443, UUID: "abc-123"})
	ob := outbound0(t, cfg)
	ss := ob["streamSettings"].(map[string]interface{})
	if ss["security"] != "tls" {
		t.Fatalf("legacy security = %v, want tls", ss["security"])
	}
	tls := ss["tlsSettings"].(map[string]interface{})
	if tls["serverName"] != "example.com" {
		t.Errorf("legacy serverName = %v, want example.com", tls["serverName"])
	}
	alpn := tls["alpn"].([]interface{})
	if len(alpn) != 1 || alpn[0] != "h2" {
		t.Errorf("legacy alpn = %v, want [h2]", alpn)
	}
	u0 := ob["settings"].(map[string]interface{})["vnext"].([]interface{})[0].(map[string]interface{})["users"].([]interface{})[0].(map[string]interface{})
	if _, ok := u0["flow"]; ok {
		t.Errorf("legacy must not set flow, got %v", u0["flow"])
	}
}

func TestGenerateConfig_MissingTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewXrayService(filepath.Join(tmpDir, "nonexistent"), filepath.Join(tmpDir, "out"))
	if err := svc.GenerateConfig(vpnconfig.Server{}); err == nil {
		t.Error("expected error for missing template")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd server && go test ./internal/service/ -run TestGenerateConfig`
Expected: FAIL — current `GenerateConfig` uses `{{...}}` replacement and produces invalid JSON / no `realitySettings`.

- [ ] **Step 3: Rewrite `xray.go`**

Replace `server/internal/service/xray.go` entirely:

```go
// internal/service/xray.go
package service

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

// XrayService handles Xray configuration generation
type XrayService struct {
	templatePath string
	outputPath   string
}

var _ XrayGenerator = (*XrayService)(nil)

func NewXrayService(templatePath, outputPath string) *XrayService {
	return &XrayService{templatePath: templatePath, outputPath: outputPath}
}

type xrayUser struct {
	ID         string `json:"id"`
	Encryption string `json:"encryption"`
	Flow       string `json:"flow,omitempty"`
}

type xrayVnext struct {
	Address string     `json:"address"`
	Port    int        `json:"port"`
	Users   []xrayUser `json:"users"`
}

type xrayReality struct {
	ServerName  string `json:"serverName,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	PublicKey   string `json:"publicKey,omitempty"`
	ShortID     string `json:"shortId,omitempty"`
}

type xrayTLS struct {
	ServerName  string   `json:"serverName,omitempty"`
	Fingerprint string   `json:"fingerprint,omitempty"`
	ALPN        []string `json:"alpn,omitempty"`
}

type xrayStream struct {
	Network         string       `json:"network"`
	Security        string       `json:"security"`
	RealitySettings *xrayReality `json:"realitySettings,omitempty"`
	TLSSettings     *xrayTLS     `json:"tlsSettings,omitempty"`
}

type xrayOutbound struct {
	Protocol       string                 `json:"protocol"`
	Settings       map[string]interface{} `json:"settings"`
	StreamSettings xrayStream             `json:"streamSettings"`
	Tag            string                 `json:"tag"`
}

func buildOutbound(s vpnconfig.Server) xrayOutbound {
	network := s.Network
	if network == "" {
		network = "tcp"
	}
	stream := xrayStream{Network: network}
	switch s.Security {
	case "reality":
		stream.Security = "reality"
		stream.RealitySettings = &xrayReality{
			ServerName:  s.SNI,
			Fingerprint: s.Fingerprint,
			PublicKey:   s.PublicKey,
			ShortID:     s.ShortID,
		}
	case "tls":
		stream.Security = "tls"
		serverName := s.SNI
		if serverName == "" {
			serverName = s.Address
		}
		stream.TLSSettings = &xrayTLS{
			ServerName:  serverName,
			Fingerprint: s.Fingerprint,
			ALPN:        s.ALPN,
		}
	default: // legacy: empty security -> TLS to address with alpn h2, no flow
		stream.Security = "tls"
		stream.TLSSettings = &xrayTLS{ServerName: s.Address, ALPN: []string{"h2"}}
	}
	return xrayOutbound{
		Protocol: "vless",
		Settings: map[string]interface{}{
			"vnext": []xrayVnext{{
				Address: s.Address,
				Port:    s.Port,
				Users:   []xrayUser{{ID: s.UUID, Encryption: "none", Flow: s.Flow}},
			}},
		},
		StreamSettings: stream,
		Tag:            "proxy-out",
	}
}

// GenerateConfig parses the (valid-JSON) template and replaces outbounds
// with a single proxy-out outbound built from the server's stream params.
func (s *XrayService) GenerateConfig(server vpnconfig.Server) error {
	template, err := os.ReadFile(s.templatePath)
	if err != nil {
		return fmt.Errorf("read template: %w", err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(template, &cfg); err != nil {
		return fmt.Errorf("parse template: %w", err)
	}
	cfg["outbounds"] = []xrayOutbound{buildOutbound(server)}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(s.outputPath, append(out, '\n'), 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd server && go test ./internal/service/ -v -run TestGenerateConfig`
Expected: PASS (Reality, TLS, Legacy, MissingTemplate).

- [ ] **Step 5: Commit**

```bash
git add server/internal/service/xray.go server/internal/service/xray_test.go
git commit -m "feat(xray): generate REALITY/TLS outbound from server params"
```

---

### Task 5: Shell — `lib/xrayconf.sh` outbound/config builder

**Files:**
- Create: `router/opt/vpn-director/lib/xrayconf.sh`
- Test: `router/test/unit/xrayconf.bats` (create)

- [ ] **Step 1: Write the failing tests**

Create `router/test/unit/xrayconf.bats` (match the `load` line used by other files in `router/test/unit/`):

```bash
#!/usr/bin/env bats
load '../test_helper.bash'

setup() {
    source "$LIB_DIR/xrayconf.sh"
}

@test "build_outbound: reality -> realitySettings + flow" {
    server='{"address":"1.2.3.4","port":443,"uuid":"u1","security":"reality","network":"tcp","flow":"xtls-rprx-vision","sni":"cdn.example.com","fingerprint":"firefox","public_key":"PBK","short_id":"sid1"}'
    run xrayconf_build_outbound <<< "$server"
    [ "$status" -eq 0 ]
    [ "$(printf '%s' "$output" | jq -r '.streamSettings.security')" = "reality" ]
    [ "$(printf '%s' "$output" | jq -r '.streamSettings.realitySettings.publicKey')" = "PBK" ]
    [ "$(printf '%s' "$output" | jq -r '.streamSettings.realitySettings.serverName')" = "cdn.example.com" ]
    [ "$(printf '%s' "$output" | jq -r '.settings.vnext[0].users[0].flow')" = "xtls-rprx-vision" ]
}

@test "build_outbound: tls -> tlsSettings serverName from sni" {
    server='{"address":"1.2.3.4","port":443,"uuid":"u1","security":"tls","sni":"host.example.com","fingerprint":"chrome"}'
    run xrayconf_build_outbound <<< "$server"
    [ "$status" -eq 0 ]
    [ "$(printf '%s' "$output" | jq -r '.streamSettings.security')" = "tls" ]
    [ "$(printf '%s' "$output" | jq -r '.streamSettings.tlsSettings.serverName')" = "host.example.com" ]
}

@test "build_outbound: legacy (no security) -> tls alpn h2, no flow" {
    server='{"address":"example.com","port":443,"uuid":"u1"}'
    run xrayconf_build_outbound <<< "$server"
    [ "$status" -eq 0 ]
    [ "$(printf '%s' "$output" | jq -r '.streamSettings.security')" = "tls" ]
    [ "$(printf '%s' "$output" | jq -r '.streamSettings.tlsSettings.serverName')" = "example.com" ]
    [ "$(printf '%s' "$output" | jq -r '.streamSettings.tlsSettings.alpn[0]')" = "h2" ]
    [ "$(printf '%s' "$output" | jq -r '.settings.vnext[0].users[0] | has("flow")')" = "false" ]
}

@test "generate: replaces outbounds and preserves inbounds/routing" {
    tmpl="$BATS_TEST_TMPDIR/t.json"
    printf '%s' '{"inbounds":[{"tag":"tproxy-in"},{"tag":"socks-in"}],"outbounds":[],"routing":{"rules":[{"outboundTag":"proxy-out"}]}}' > "$tmpl"
    server='{"address":"1.2.3.4","port":443,"uuid":"u1","security":"reality","sni":"s","public_key":"PBK","short_id":"sid"}'
    run xrayconf_generate "$tmpl" <<< "$server"
    [ "$status" -eq 0 ]
    [ "$(printf '%s' "$output" | jq -r '.outbounds[0].streamSettings.security')" = "reality" ]
    [ "$(printf '%s' "$output" | jq -r '.outbounds | length')" = "1" ]
    # inbounds/routing from the template must be preserved untouched
    [ "$(printf '%s' "$output" | jq -r '.inbounds | length')" = "2" ]
    [ "$(printf '%s' "$output" | jq -r '.routing.rules[0].outboundTag')" = "proxy-out" ]
}
```

> Note: tests invoke the shell functions directly (`run xrayconf_build_outbound <<< "$server"`), NOT via `run bash -c "… | xrayconf_build_outbound"` — a `bash -c` child shell does not inherit functions sourced in `setup()`, so that form fails with `command not found`. The herestring also avoids re-quoting the JSON (`$`/apostrophe safe). `setup()` sources only `xrayconf.sh`, which must stay self-contained (no `common.sh`/mock deps); if a future test here needs the `test_helper.bash` mocks, switch to a `load_*`-style helper instead of overriding `setup()`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `bats router/test/unit/xrayconf.bats`
Expected: FAIL — `xrayconf.sh` does not exist (source error).

- [ ] **Step 3: Create `lib/xrayconf.sh`**

```bash
#!/usr/bin/env bash
###############################################################################
# lib/xrayconf.sh - Build Xray proxy-out outbound + config.json from a server
# JSON object (as stored in servers.json). Pure jq transforms; no side effects
# on source. Used by configure.sh; unit-tested via bats.
###############################################################################

# xrayconf_build_outbound: reads one server JSON object on stdin,
# prints the proxy-out outbound JSON object on stdout.
xrayconf_build_outbound() {
    jq '
      def trimempty: with_entries(select(.value != null and .value != "" and .value != []));
      ((.network // "") | if . == "" then "tcp" else . end) as $net
      | (.security // "") as $sec
      | {
          protocol: "vless",
          settings: { vnext: [ {
            address: .address,
            port: .port,
            users: [ ( { id: .uuid, encryption: "none" }
                       + (if (.flow // "") != "" then { flow: .flow } else {} end) ) ]
          } ] },
          streamSettings: (
            if $sec == "reality" then
              { network: $net, security: "reality",
                realitySettings: ( { serverName: .sni, fingerprint: .fingerprint,
                                     publicKey: .public_key, shortId: .short_id } | trimempty ) }
            elif $sec == "tls" then
              ( (if (.sni // "") != "" then .sni else .address end) as $sn
                | { network: $net, security: "tls",
                    tlsSettings: ( { serverName: $sn, fingerprint: .fingerprint, alpn: .alpn } | trimempty ) } )
            else
              { network: $net, security: "tls",
                tlsSettings: { alpn: ["h2"], serverName: .address } }
            end
          ),
          tag: "proxy-out"
        }
    '
}

# xrayconf_generate <template_path>: reads one server JSON object on stdin,
# prints the full config.json (template with outbounds replaced) on stdout.
xrayconf_generate() {
    local template="$1"
    local server_json outbound
    server_json="$(cat)"
    outbound="$(printf '%s' "$server_json" | xrayconf_build_outbound)" || return 1
    jq --argjson ob "$outbound" '.outbounds = [$ob]' "$template"
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `bats router/test/unit/xrayconf.bats`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add router/opt/vpn-director/lib/xrayconf.sh router/test/unit/xrayconf.bats
git commit -m "feat(xrayconf): jq builder for REALITY/TLS outbound + config"
```

---

### Task 6: Shell — extract query params in `import_server_list.sh`

**Files:**
- Modify: `router/opt/vpn-director/import_server_list.sh:62-101` (parser), `:181-272` (builder)
- Test: `router/test/import_server_list.bats`

- [ ] **Step 1: Write the failing tests**

Add to `router/test/import_server_list.bats` (it already sources the script with `IMPORT_TEST_MODE=1`; match the existing source/setup pattern in that file):

```bash
@test "parse_vless_uri extracts reality stream params" {
    # Real subscription format includes headerType=none before type=tcp — guards the
    # `_vless_query_get type` lookup against matching `headerType`.
    uri='vless://uuid@1.2.3.4:443?security=reality&encryption=none&fp=firefox&headerType=none&type=tcp&flow=xtls-rprx-vision&sni=cdn3-87.yahoo.com&pbk=PBKEY&sid=55e6#NL'
    run parse_vless_uri "$uri"
    [ "$status" -eq 0 ]
    # fields: server|port|uuid|name|security|network|flow|sni|fp|pbk|sid|alpn
    [ "$(printf '%s' "$output" | cut -d'|' -f5)" = "reality" ]
    [ "$(printf '%s' "$output" | cut -d'|' -f6)" = "tcp" ]
    [ "$(printf '%s' "$output" | cut -d'|' -f7)" = "xtls-rprx-vision" ]
    [ "$(printf '%s' "$output" | cut -d'|' -f8)" = "cdn3-87.yahoo.com" ]
    [ "$(printf '%s' "$output" | cut -d'|' -f9)" = "firefox" ]
    [ "$(printf '%s' "$output" | cut -d'|' -f10)" = "PBKEY" ]
    [ "$(printf '%s' "$output" | cut -d'|' -f11)" = "55e6" ]
}

@test "parse_vless_uri keeps core fields without params" {
    run parse_vless_uri 'vless://uuid@1.2.3.4:443#Name'
    [ "$status" -eq 0 ]
    [ "$(printf '%s' "$output" | cut -d'|' -f1)" = "1.2.3.4" ]
    [ "$(printf '%s' "$output" | cut -d'|' -f3)" = "uuid" ]
    [ "$(printf '%s' "$output" | cut -d'|' -f5)" = "" ]
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `bats router/test/import_server_list.bats`
Expected: FAIL — output has only 4 pipe fields; fields 5-11 are empty.

- [ ] **Step 3: Add a query mini-parser and extend `parse_vless_uri`**

In `router/opt/vpn-director/import_server_list.sh`, add this helper just above `parse_vless_uri()`:

```bash
# Extract a query parameter value (no external tools)
# _vless_query_get <query_string> <key> -> prints value (empty if absent)
_vless_query_get() {
    local q="&$1&" v
    case "$q" in
        *"&$2="*)
            v="${q#*"&$2="}"
            printf '%s' "${v%%&*}"
            ;;
    esac
}
```

Also harden the existing name extraction so a URI **without** a `#fragment` doesn't capture the query string as the name. Replace the bare `raw_name="${rest##*#}"` line with a guard:

```bash
    if [[ "$rest" == *#* ]]; then
        raw_name="${rest##*#}"
    else
        raw_name=""
    fi
```

(The existing `if [[ -z "$name" ]]; then name="$server"; fi` fallback then yields the server hostname. Real subscription URIs always include `#name`, so this only affects malformed/fragment-less input.)

Then in `parse_vless_uri()`, after `rest="${rest#*@}"` and the existing `server_port="${rest%%\?*}"` line, add query capture and param extraction, and replace the final `printf` (line 100) so it emits all 12 fields:

```bash
    # Extract query string (after ?), then stream params
    if [[ "$rest" == *\?* ]]; then
        query="${rest#*\?}"
    else
        query=""
    fi

    security=$(_vless_query_get "$query" security)
    network=$(_vless_query_get "$query" type)
    flow=$(_vless_query_get "$query" flow)
    sni=$(_vless_query_get "$query" sni)
    fp=$(_vless_query_get "$query" fp)
    pbk=$(_vless_query_get "$query" pbk)
    sid=$(_vless_query_get "$query" sid)
    alpn=$(_vless_query_get "$query" alpn)

    # Fallback: if name is empty after filtering, use server hostname
    if [[ -z "$name" ]]; then
        name="$server"
    fi

    printf '%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s\n' \
        "$server" "$port" "$uuid" "$name" \
        "$security" "$network" "$flow" "$sni" "$fp" "$pbk" "$sid" "$alpn"
```

(Remove the old `if [[ -z "$name" ]]` block and old final `printf '%s|%s|%s|%s\n' ...` — they are replaced by the lines above.)

- [ ] **Step 4: Run parser tests to verify they pass**

Run: `bats router/test/import_server_list.bats`
Expected: PASS for the two new parser tests (the existing 4-field tests still pass — fields 1-4 unchanged).

- [ ] **Step 5: Write the failing builder test**

Add to `router/test/import_server_list.bats`:

```bash
@test "step_parse_and_save_servers writes reality params to servers.json" {
    tmp_data="$BATS_TEST_TMPDIR/data"
    mkdir -p "$tmp_data"
    # get_data_dir reads VPD_CONFIG/VPD_TEMPLATE; override to a temp config
    cfg="$BATS_TEST_TMPDIR/vpn-director.json"
    printf '{"data_dir":"%s"}' "$tmp_data" > "$cfg"
    VPD_CONFIG="$cfg"
    VLESS_SERVERS='vless://uuid@1.2.3.4:443?security=reality&flow=xtls-rprx-vision&sni=cdn.example.com&pbk=PBK&sid=sid1&type=tcp#NL'
    run step_parse_and_save_servers
    [ "$status" -eq 0 ]
    out="$tmp_data/servers.json"
    [ "$(jq -r '.[0].security' "$out")" = "reality" ]
    [ "$(jq -r '.[0].flow' "$out")" = "xtls-rprx-vision" ]
    [ "$(jq -r '.[0].public_key' "$out")" = "PBK" ]
    [ "$(jq -r '.[0].short_id' "$out")" = "sid1" ]
    [ "$(jq -r '.[0].sni' "$out")" = "cdn.example.com" ]
    [ "$(jq -r '.[0] | has("alpn")' "$out")" = "false" ]
}
```

(`1.2.3.4` resolves via the test `resolve_ip`/mock `nslookup` to a literal IP; the `mocks/nslookup` fixture returns IPs. If `resolve_ip` of a literal IP returns it directly, no mock needed.)

- [ ] **Step 6: Run builder test to verify it fails**

Run: `bats router/test/import_server_list.bats -f "writes reality params"`
Expected: FAIL — builder still emits only `address/port/uuid/name/ips`.

- [ ] **Step 7: Rewrite the builder `step_parse_and_save_servers`**

Replace the body of `step_parse_and_save_servers` (the parse loop + JSON assembly, lines ~190-254) with this single-pipeline version using `jq -s`:

```bash
    # Parse servers, resolve IPs, emit one JSON object per server, slurp to array
    printf '%s\n' "$VLESS_SERVERS" | grep '^vless://' | while IFS= read -r uri; do
        log -l DEBUG "URI: ${uri%%#*}"

        parsed=$(parse_vless_uri "$uri")
        server=$(printf '%s' "$parsed" | cut -d'|' -f1)
        port=$(printf '%s' "$parsed" | cut -d'|' -f2)
        uuid=$(printf '%s' "$parsed" | cut -d'|' -f3)
        name=$(printf '%s' "$parsed" | cut -d'|' -f4)
        security=$(printf '%s' "$parsed" | cut -d'|' -f5)
        network=$(printf '%s' "$parsed" | cut -d'|' -f6)
        flow=$(printf '%s' "$parsed" | cut -d'|' -f7)
        sni=$(printf '%s' "$parsed" | cut -d'|' -f8)
        fp=$(printf '%s' "$parsed" | cut -d'|' -f9)
        pbk=$(printf '%s' "$parsed" | cut -d'|' -f10)
        sid=$(printf '%s' "$parsed" | cut -d'|' -f11)
        alpn=$(printf '%s' "$parsed" | cut -d'|' -f12)

        if [[ -z "$server" ]] || [[ -z "$port" ]] || [[ -z "$uuid" ]]; then
            log -l WARN "Skipping invalid URI (missing server/port/uuid)"
            continue
        fi
        if ! printf '%s' "$port" | grep -qE '^[0-9]+$'; then
            log -l WARN "Skipping $server: invalid port '$port'"
            continue
        fi

        ips_raw=$(resolve_ip -a -q "$server" 2>/dev/null) || ips_raw=""
        if [[ -z "$ips_raw" ]]; then
            log -l WARN "Cannot resolve $server, skipping"
            continue
        fi
        ips_oneline=$(printf '%s' "$ips_raw" | tr '\n' ',' | sed 's/,$//')
        printf "  %s (%s) -> %s\n" "$name" "$server" "$ips_oneline" >&2

        ips_json=$(printf '%s\n' "$ips_raw" | jq -R 'select(length > 0)' | jq -s .)
        if [[ -n "$alpn" ]]; then
            alpn_json=$(printf '%s' "$alpn" | tr ',' '\n' | jq -R 'select(length > 0)' | jq -s .)
        else
            alpn_json='[]'
        fi

        jq -c -n \
            --arg address "$server" --argjson port "$port" --arg uuid "$uuid" --arg name "$name" \
            --argjson ips "$ips_json" \
            --arg security "$security" --arg network "$network" --arg flow "$flow" \
            --arg sni "$sni" --arg fingerprint "$fp" --arg public_key "$pbk" --arg short_id "$sid" \
            --argjson alpn "$alpn_json" \
            '{address:$address, port:$port, uuid:$uuid, name:$name, ips:$ips,
              security:$security, network:$network, flow:$flow, sni:$sni,
              fingerprint:$fingerprint, public_key:$public_key, short_id:$short_id, alpn:$alpn}
             | with_entries(select(.value != null and .value != "" and .value != []))'
    done | jq -s '.' > "$SERVERS_FILE"
```

Keep the existing JSON validation / count / "Saved N servers" block that follows (lines ~256-271) unchanged.

- [ ] **Step 8: Run all import tests to verify they pass**

Run: `bats router/test/import_server_list.bats`
Expected: PASS (existing + new parser + builder tests).

- [ ] **Step 9: Commit**

```bash
git add router/opt/vpn-director/import_server_list.sh router/test/import_server_list.bats
git commit -m "feat(import): capture REALITY/TLS stream params into servers.json"
```

---

### Task 7: Shell — wire `configure.sh` to the new generator

**Files:**
- Modify: `router/opt/vpn-director/configure.sh` (source line, `:29-31`, `:138-141`, `:426-434`)

- [ ] **Step 1: Source the new lib**

`configure.sh` is self-contained — it does **not** source `lib/common.sh` and has **no** `SCRIPT_DIR`; it only defines `VPD_DIR="/opt/vpn-director"` (line 23). Under `set -euo pipefail`, sourcing via an undefined `$SCRIPT_DIR` would abort with `unbound variable`. Add the source right after the `VPD_DIR=...` line, using `$VPD_DIR`:

```bash
. "$VPD_DIR/lib/xrayconf.sh"
```

(`xrayconf.sh` is self-contained — pure `jq`, no `common.sh` dependency — so sourcing it alone is safe.)

- [ ] **Step 2: Add a global for the selected server JSON**

After the `SELECTED_SERVER_UUID=""` line (around line 31), add:

```bash
SELECTED_SERVER_JSON=""
```

- [ ] **Step 3: Capture the selected server object**

In `step_select_xray_server`, right after the `SELECTED_SERVER_UUID=$(jq -r ".[$idx].uuid" "$SERVERS_FILE")` line (around line 140), add:

```bash
    SELECTED_SERVER_JSON=$(jq -c ".[$idx]" "$SERVERS_FILE")
```

- [ ] **Step 4: Replace the sed-based Xray config generation**

In `step_generate_configs`, replace the `sed ... > "$XRAY_CONFIG_DIR/config.json"` block (lines 429-433) with:

```bash
    printf '%s' "$SELECTED_SERVER_JSON" \
        | xrayconf_generate "$XRAY_CONFIG_DIR/config.json.template" \
        > "$XRAY_CONFIG_DIR/config.json"
```

- [ ] **Step 5: Verify wiring (lint + smoke)**

Run: `bash -n router/opt/vpn-director/configure.sh` (syntax check).
Then smoke-test the generator path with a fixture (proves source + call path):

```bash
bash -c '
  set -e
  source router/opt/vpn-director/lib/xrayconf.sh
  printf "%s" "{\"address\":\"1.2.3.4\",\"port\":443,\"uuid\":\"u\",\"security\":\"reality\",\"sni\":\"s\",\"public_key\":\"PBK\",\"short_id\":\"sid\"}" \
    | xrayconf_generate router/opt/etc/xray/config.json.template \
    | jq -e ".outbounds[0].streamSettings.security == \"reality\""
'
```
Expected: `bash -n` clean; smoke prints `true`.

- [ ] **Step 6: Commit**

```bash
git add router/opt/vpn-director/configure.sh
git commit -m "feat(configure): generate Xray config via xrayconf from selected server"
```

---

### Task 8: Install plumbing + docs + migration note

**Files:**
- Modify: `install.sh` (download list ~141-147)
- Modify: `server/internal/updater/downloader.go` (`scriptFiles`)
- Modify: `.claude/rules/xray-tproxy.md`
- Modify: `CLAUDE.md` (architecture table row)

- [ ] **Step 1: Add `lib/xrayconf.sh` to install.sh**

In `install.sh` `download_scripts()`, add to the list (after `lib/tproxy.sh`, line 146):

```bash
        "router/opt/vpn-director/lib/xrayconf.sh" \
```

- [ ] **Step 2: Add `lib/xrayconf.sh` to downloader.go**

In `server/internal/updater/downloader.go` `scriptFiles`, add after the `lib/tproxy.sh` entry:

```go
	"router/opt/vpn-director/lib/xrayconf.sh",
```

- [ ] **Step 3: Build Go to confirm no breakage**

Run: `cd server && go build ./... && go vet ./...`
Expected: clean.

- [ ] **Step 4: Update docs**

In `.claude/rules/xray-tproxy.md`, under "How It Works" / config generation, add a note:

```markdown
## Outbound Generation (REALITY/TLS)

`config.json.template` is valid JSON with an empty `outbounds: []`. The proxy-out outbound
is generated per selected server from `servers.json` stream params and injected as `outbounds[0]`:

- Shell: `lib/xrayconf.sh` (`xrayconf_generate`, jq) — used by `configure.sh`.
- Go: `service/xray.go` (`buildOutbound` + `encoding/json`) — used by bot wizard and `/xray`.

Per-server params (parsed from the VLESS URI): `security` (`reality`|`tls`), `network`, `flow`,
`sni`, `fingerprint` (fp), `public_key` (pbk), `short_id` (sid), `alpn`.

- `security=reality` -> `realitySettings { serverName, fingerprint, publicKey, shortId }`, user `flow`.
- `security=tls` -> `tlsSettings { serverName (sni||address), fingerprint?, alpn? }`.
- empty `security` -> legacy `tlsSettings { alpn:["h2"], serverName:address }`, no flow.

**Migration after upgrade:** the previously generated `/opt/etc/xray/config.json` stays on disk as
plain TLS until regenerated, so Xray — and the Telegram bot, which proxies its API connection
through it — cannot connect to a REALITY server until the user acts:

1. Re-run `/import` (or, over SSH if the bot is unreachable through the broken proxy:
   `/opt/vpn-director/import_server_list.sh`) so params land in `servers.json`.
2. Re-select the server (`/configure` wizard or `/xray`, or over SSH `/opt/vpn-director/configure.sh`)
   to regenerate `config.json`.
```

In `CLAUDE.md`, add a row to the Architecture table after the `lib/tproxy.sh` row:

```markdown
| `router/opt/vpn-director/lib/xrayconf.sh` | Build Xray outbound (REALITY/TLS) + config.json from a server |
```

- [ ] **Step 5: Commit**

```bash
git add install.sh server/internal/updater/downloader.go .claude/rules/xray-tproxy.md CLAUDE.md
git commit -m "docs: REALITY support, install lib/xrayconf.sh, migration note"
```

---

### Task 9: Full test sweep

- [ ] **Step 1: Go**

Run: `cd server && go test ./...`
Expected: PASS.

- [ ] **Step 2: Bats**

Run: `bats router/test/ router/test/unit/ router/test/integration/`
Expected: PASS.

- [ ] **Step 3: End-to-end generation sanity (real subscription line)**

```bash
bash -c '
  source router/opt/vpn-director/lib/xrayconf.sh
  srv="{\"address\":\"162.249.126.77\",\"port\":443,\"uuid\":\"9ca8\",\"security\":\"reality\",\"network\":\"tcp\",\"flow\":\"xtls-rprx-vision\",\"sni\":\"cdn3-87.yahoo.com\",\"fingerprint\":\"firefox\",\"public_key\":\"CMkW1axrhEXoiJ6anMz9XEjlfqlAtEZya7L0b5ZPMyw\",\"short_id\":\"55e6d9bd269aac46\"}"
  printf "%s" "$srv" | xrayconf_generate router/opt/etc/xray/config.json.template | jq .outbounds[0]
'
```
Expected: outbound with `security:"reality"`, `realitySettings` populated, user `flow:"xtls-rprx-vision"`, valid JSON.

- [ ] **Step 4: (No commit — verification only.)**

---

## Self-Review

**Spec coverage:**
- §1 data model → Tasks 2, 3 (Go structs), 6 (shell servers.json). ✓
- §2 parsers → Task 2 (Go), Task 6 (shell), Task 3 (conversion sites). ✓
- §3 generation (mechanism B, valid-JSON template, replace outbounds[0]) → Task 1 (template), 4 (Go), 5 (shell lib), 7 (configure wiring). ✓
- §4 backward compat / migration → Task 4 (legacy test), Task 8 (migration doc). ✓
- §5 testing → every Task is TDD; Task 9 full sweep. ✓
- Files-touched list → all covered, plus install plumbing (Task 8). ✓

**Placeholder scan:** No TBD/TODO; every code step has complete code. ✓

**Type/name consistency:** `xrayconf_build_outbound`/`xrayconf_generate` (Tasks 5,7); `ToVPNConfig` (Task 3, used Task 3); `buildOutbound`/`GenerateConfig` (Task 4); servers.json keys `security/network/flow/sni/fingerprint/public_key/short_id/alpn` consistent across Go structs, jq filter, and shell builder. Pipe field order (1-12) consistent between Task 6 parser output and builder cut. ✓

**Note:** `configure.sh` line 118/454 use `.ip` (singular) vs servers.json `.ips` — a pre-existing latent issue, out of scope; not touched.
