package vless

import (
	"encoding/base64"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type Server struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	IP      string `json:"ip"`
}

func ParseURI(uri string) (*Server, error) {
	if !strings.HasPrefix(uri, "vless://") {
		return nil, errors.New("not a vless URI")
	}

	rest := strings.TrimPrefix(uri, "vless://")

	// Extract name (after #)
	name := ""
	if idx := strings.LastIndex(rest, "#"); idx != -1 {
		name, _ = url.QueryUnescape(rest[idx+1:])
		rest = rest[:idx]
	}

	// Remove query params
	if idx := strings.Index(rest, "?"); idx != -1 {
		rest = rest[:idx]
	}

	// Extract UUID (before @)
	atIdx := strings.Index(rest, "@")
	if atIdx == -1 {
		return nil, errors.New("missing @ in URI")
	}
	uuid := rest[:atIdx]
	rest = rest[atIdx+1:]

	// Extract server:port
	// Handle IPv6 addresses in brackets
	var address string
	var portStr string

	if strings.HasPrefix(rest, "[") {
		// IPv6 address
		closeBracket := strings.Index(rest, "]")
		if closeBracket == -1 {
			return nil, errors.New("invalid IPv6 address format")
		}
		address = rest[:closeBracket+1]
		rest = rest[closeBracket+1:]
		if !strings.HasPrefix(rest, ":") {
			return nil, errors.New("missing port in URI")
		}
		portStr = rest[1:]
	} else {
		// IPv4 or hostname
		colonIdx := strings.LastIndex(rest, ":")
		if colonIdx == -1 {
			return nil, errors.New("missing port in URI")
		}
		address = rest[:colonIdx]
		portStr = rest[colonIdx+1:]
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, errors.New("invalid port")
	}

	if address == "" || uuid == "" {
		return nil, errors.New("missing required fields")
	}

	if name == "" {
		name = address
	}

	return &Server{
		Address: address,
		Port:    port,
		UUID:    uuid,
		Name:    name,
	}, nil
}

func (s *Server) ResolveIP() error {
	ips, err := net.LookupIP(s.Address)
	if err != nil {
		return err
	}
	for _, ip := range ips {
		if ipv4 := ip.To4(); ipv4 != nil {
			s.IP = ipv4.String()
			return nil
		}
	}
	if len(ips) > 0 {
		s.IP = ips[0].String()
	}
	return nil
}

func DecodeSubscription(encoded string) ([]*Server, []error) {
	encoded = strings.TrimSpace(encoded)

	var decoded []byte
	var decodeErr error

	// Try all base64 variants (padded and raw, standard and URL-safe)
	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.RawURLEncoding,
	}

	for _, enc := range encodings {
		decoded, decodeErr = enc.DecodeString(encoded)
		if decodeErr == nil {
			break
		}
	}

	if decodeErr != nil {
		return nil, []error{errors.New("failed to decode base64")}
	}

	var servers []*Server
	var parseErrors []error

	lines := strings.Split(string(decoded), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "vless://") {
			continue
		}

		server, err := ParseURI(line)
		if err != nil {
			parseErrors = append(parseErrors, err)
			continue
		}
		servers = append(servers, server)
	}

	return servers, parseErrors
}
