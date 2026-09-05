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
		ALPN: []string{"h2", "http/1.1"},
	})
	ss := outbound0(t, cfg)["streamSettings"].(map[string]interface{})
	if ss["security"] != "tls" {
		t.Fatalf("security = %v, want tls", ss["security"])
	}
	tls := ss["tlsSettings"].(map[string]interface{})
	if tls["serverName"] != "host.example.com" || tls["fingerprint"] != "chrome" {
		t.Errorf("tlsSettings wrong: %v", tls)
	}
	alpn, ok := tls["alpn"].([]interface{})
	if !ok || len(alpn) != 2 || alpn[0] != "h2" || alpn[1] != "http/1.1" {
		t.Errorf("tls alpn = %v, want [h2 http/1.1]", tls["alpn"])
	}
	if _, ok := ss["realitySettings"]; ok {
		t.Errorf("tls outbound must not contain realitySettings")
	}
}

func TestGenerateConfig_Legacy(t *testing.T) {
	cfg := generate(t, vpnconfig.Server{Address: "example.com", Port: 443, UUID: "abc-123", Flow: "xtls-rprx-vision"})
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

func TestGenerateConfig_UnsupportedNetwork(t *testing.T) {
	tmpDir := t.TempDir()
	templatePath := filepath.Join(tmpDir, "config.json.template")
	if err := os.WriteFile(templatePath, []byte(testTemplate), 0644); err != nil {
		t.Fatal(err)
	}
	svc := NewXrayService(templatePath, filepath.Join(tmpDir, "config.json"))
	if err := svc.GenerateConfig(vpnconfig.Server{Address: "1.2.3.4", Port: 443, UUID: "u", Network: "ws", Security: "reality"}); err == nil {
		t.Error("expected error for unsupported network 'ws'")
	}
}

func TestGenerateConfig_UnsupportedSecurity(t *testing.T) {
	tmpDir := t.TempDir()
	templatePath := filepath.Join(tmpDir, "config.json.template")
	if err := os.WriteFile(templatePath, []byte(testTemplate), 0644); err != nil {
		t.Fatal(err)
	}
	svc := NewXrayService(templatePath, filepath.Join(tmpDir, "config.json"))
	if err := svc.GenerateConfig(vpnconfig.Server{Address: "1.2.3.4", Port: 443, UUID: "u", Security: "xtls"}); err == nil {
		t.Error("expected error for unsupported security 'xtls'")
	}
}

func TestGenerateConfig_RealityMissingRequiredFields(t *testing.T) {
	mkSvc := func() *XrayService {
		tmpDir := t.TempDir()
		templatePath := filepath.Join(tmpDir, "config.json.template")
		if err := os.WriteFile(templatePath, []byte(testTemplate), 0644); err != nil {
			t.Fatal(err)
		}
		return NewXrayService(templatePath, filepath.Join(tmpDir, "config.json"))
	}
	base := vpnconfig.Server{
		Address: "1.2.3.4", Port: 443, UUID: "u", Security: "reality",
		SNI: "s.example.com", Fingerprint: "chrome", PublicKey: "PBK", ShortID: "sid",
	}
	for _, f := range []string{"public_key", "sni", "fingerprint"} {
		s := base
		switch f {
		case "public_key":
			s.PublicKey = ""
		case "sni":
			s.SNI = ""
		case "fingerprint":
			s.Fingerprint = ""
		}
		if err := mkSvc().GenerateConfig(s); err == nil {
			t.Errorf("expected error when reality %s is empty", f)
		}
	}
	// shortId is optional: reality without it must still generate.
	s := base
	s.ShortID = ""
	if err := mkSvc().GenerateConfig(s); err != nil {
		t.Errorf("reality without shortId should succeed, got %v", err)
	}
}

func TestGenerateConfig_KeepsTemplateLogSection(t *testing.T) {
	tmpDir := t.TempDir()
	templatePath := filepath.Join(tmpDir, "config.json.template")
	outputPath := filepath.Join(tmpDir, "config.json")
	tmpl := `{"log":{"loglevel":"warning","access":"none","error":"/tmp/xray-error.log"},"inbounds":[],"outbounds":[],"routing":{}}`
	if err := os.WriteFile(templatePath, []byte(tmpl), 0644); err != nil {
		t.Fatal(err)
	}

	server := vpnconfig.Server{Address: "1.2.3.4", Port: 443, UUID: "u1", Security: "tls"}
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
	logSection, ok := cfg["log"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected log section to be preserved, got %v", cfg["log"])
	}
	if logSection["error"] != "/tmp/xray-error.log" || logSection["access"] != "none" {
		t.Errorf("log section altered: %v", logSection)
	}
}
