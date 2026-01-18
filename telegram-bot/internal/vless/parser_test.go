package vless

import (
	"encoding/base64"
	"testing"
)

func TestParseURI_ValidURI(t *testing.T) {
	uri := "vless://550e8400-e29b-41d4-a716-446655440000@server.example.com:443?encryption=none#MyServer"

	server, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if server.UUID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("expected UUID '550e8400-e29b-41d4-a716-446655440000', got '%s'", server.UUID)
	}

	if server.Address != "server.example.com" {
		t.Errorf("expected Address 'server.example.com', got '%s'", server.Address)
	}

	if server.Port != 443 {
		t.Errorf("expected Port 443, got %d", server.Port)
	}

	if server.Name != "MyServer" {
		t.Errorf("expected Name 'MyServer', got '%s'", server.Name)
	}
}

func TestParseURI_URLEncodedName(t *testing.T) {
	uri := "vless://uuid@server.com:443#My%20Server%20Name"

	server, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if server.Name != "My Server Name" {
		t.Errorf("expected Name 'My Server Name', got '%s'", server.Name)
	}
}

func TestParseURI_NoName(t *testing.T) {
	uri := "vless://uuid@server.example.com:443"

	server, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Name should default to address when not specified
	if server.Name != "server.example.com" {
		t.Errorf("expected Name 'server.example.com', got '%s'", server.Name)
	}
}

func TestParseURI_NoQueryParams(t *testing.T) {
	uri := "vless://uuid@server.example.com:443#ServerName"

	server, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if server.Address != "server.example.com" {
		t.Errorf("expected Address 'server.example.com', got '%s'", server.Address)
	}
}

func TestParseURI_NotVlessScheme(t *testing.T) {
	uri := "vmess://uuid@server.example.com:443"

	_, err := ParseURI(uri)
	if err == nil {
		t.Fatal("expected error for non-vless URI")
	}
}

func TestParseURI_MissingAt(t *testing.T) {
	uri := "vless://uuidserver.example.com:443"

	_, err := ParseURI(uri)
	if err == nil {
		t.Fatal("expected error for URI missing @")
	}
}

func TestParseURI_MissingPort(t *testing.T) {
	uri := "vless://uuid@server.example.com"

	_, err := ParseURI(uri)
	if err == nil {
		t.Fatal("expected error for URI missing port")
	}
}

func TestParseURI_InvalidPort(t *testing.T) {
	uri := "vless://uuid@server.example.com:notaport"

	_, err := ParseURI(uri)
	if err == nil {
		t.Fatal("expected error for invalid port")
	}
}

func TestParseURI_EmptyUUID(t *testing.T) {
	uri := "vless://@server.example.com:443"

	_, err := ParseURI(uri)
	if err == nil {
		t.Fatal("expected error for empty UUID")
	}
}

func TestParseURI_EmptyAddress(t *testing.T) {
	uri := "vless://uuid@:443"

	_, err := ParseURI(uri)
	if err == nil {
		t.Fatal("expected error for empty address")
	}
}

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
}

func TestParseURI_IPv6Address(t *testing.T) {
	uri := "vless://uuid@[2001:db8::1]:443#IPv6Server"

	server, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if server.Address != "[2001:db8::1]" {
		t.Errorf("expected Address '[2001:db8::1]', got '%s'", server.Address)
	}

	if server.Port != 443 {
		t.Errorf("expected Port 443, got %d", server.Port)
	}
}

// Tests for DecodeSubscription

func TestDecodeSubscription_ValidBase64(t *testing.T) {
	// Two valid VLESS URIs
	rawContent := "vless://uuid1@server1.com:443#Server1\nvless://uuid2@server2.com:443#Server2"
	encoded := base64.StdEncoding.EncodeToString([]byte(rawContent))

	servers, errs := DecodeSubscription(encoded)

	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}

	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}

	if servers[0].Name != "Server1" {
		t.Errorf("expected first server name 'Server1', got '%s'", servers[0].Name)
	}

	if servers[1].Name != "Server2" {
		t.Errorf("expected second server name 'Server2', got '%s'", servers[1].Name)
	}
}

func TestDecodeSubscription_URLEncodedBase64(t *testing.T) {
	// URL-safe base64 encoding
	rawContent := "vless://uuid@server.com:443#TestServer"
	encoded := base64.URLEncoding.EncodeToString([]byte(rawContent))

	servers, errs := DecodeSubscription(encoded)

	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}

	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}

	if servers[0].Name != "TestServer" {
		t.Errorf("expected server name 'TestServer', got '%s'", servers[0].Name)
	}
}

func TestDecodeSubscription_SkipsEmptyLines(t *testing.T) {
	rawContent := "vless://uuid1@server1.com:443#Server1\n\n\nvless://uuid2@server2.com:443#Server2\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(rawContent))

	servers, errs := DecodeSubscription(encoded)

	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}

	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}
}

func TestDecodeSubscription_SkipsNonVlessLines(t *testing.T) {
	rawContent := "# Comment line\nvless://uuid@server.com:443#TestServer\nvmess://other@server.com:443"
	encoded := base64.StdEncoding.EncodeToString([]byte(rawContent))

	servers, errs := DecodeSubscription(encoded)

	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}

	if len(servers) != 1 {
		t.Fatalf("expected 1 server (only vless), got %d", len(servers))
	}
}

func TestDecodeSubscription_InvalidBase64(t *testing.T) {
	_, errs := DecodeSubscription("not-valid-base64!!!")

	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
}

func TestDecodeSubscription_InvalidVlessURI(t *testing.T) {
	rawContent := "vless://uuid@server.com:443#ValidServer\nvless://invalid-uri"
	encoded := base64.StdEncoding.EncodeToString([]byte(rawContent))

	servers, errs := DecodeSubscription(encoded)

	// Should parse the valid one and report error for invalid
	if len(servers) != 1 {
		t.Errorf("expected 1 valid server, got %d", len(servers))
	}

	if len(errs) != 1 {
		t.Errorf("expected 1 parse error, got %d", len(errs))
	}
}

func TestDecodeSubscription_EmptyContent(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(""))

	servers, errs := DecodeSubscription(encoded)

	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}

	if len(servers) != 0 {
		t.Errorf("expected 0 servers, got %d", len(servers))
	}
}

// Tests for ResolveIP

func TestResolveIP_ValidHostname(t *testing.T) {
	server := &Server{
		Address: "google.com",
		Port:    443,
		UUID:    "test",
		Name:    "Test",
	}

	err := server.ResolveIP()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if server.IP == "" {
		t.Error("expected IP to be resolved")
	}
}

func TestResolveIP_InvalidHostname(t *testing.T) {
	server := &Server{
		Address: "this-hostname-definitely-does-not-exist.invalid",
		Port:    443,
		UUID:    "test",
		Name:    "Test",
	}

	err := server.ResolveIP()
	if err == nil {
		t.Error("expected error for invalid hostname")
	}
}

func TestDecodeSubscription_RawStdBase64(t *testing.T) {
	// Raw standard base64 (without padding)
	rawContent := "vless://uuid@server.com:443#RawTest"
	encoded := base64.RawStdEncoding.EncodeToString([]byte(rawContent))

	servers, errs := DecodeSubscription(encoded)

	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}

	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}

	if servers[0].Name != "RawTest" {
		t.Errorf("expected server name 'RawTest', got '%s'", servers[0].Name)
	}
}

func TestDecodeSubscription_RawURLBase64(t *testing.T) {
	// Raw URL-safe base64 (without padding)
	rawContent := "vless://uuid@server.com:443#RawURLTest"
	encoded := base64.RawURLEncoding.EncodeToString([]byte(rawContent))

	servers, errs := DecodeSubscription(encoded)

	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}

	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}

	if servers[0].Name != "RawURLTest" {
		t.Errorf("expected server name 'RawURLTest', got '%s'", servers[0].Name)
	}
}

func TestDecodeSubscription_WhitespaceInput(t *testing.T) {
	// Input with leading/trailing whitespace and newlines
	rawContent := "vless://uuid@server.com:443#WhitespaceTest"
	encoded := "  \n\t" + base64.StdEncoding.EncodeToString([]byte(rawContent)) + "  \n\t"

	servers, errs := DecodeSubscription(encoded)

	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}

	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}

	if servers[0].Name != "WhitespaceTest" {
		t.Errorf("expected server name 'WhitespaceTest', got '%s'", servers[0].Name)
	}
}
