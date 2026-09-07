package webapi

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

// countryCodeRe matches a two-letter ISO country code in lowercase, the form
// the bot, the config template and lib/ipset.sh use.
var countryCodeRe = regexp.MustCompile(`^[a-z]{2}$`)

// handleListExcludeSets returns a handler that lists configured exclusion sets.
func handleListExcludeSets(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to load configuration")
			return
		}
		jsonOK(w, map[string]interface{}{"sets": cfg.Xray.ExcludeSets})
	}
}

// updateExcludeSetsRequest is the expected JSON body for POST /api/excludes/sets.
type updateExcludeSetsRequest struct {
	Sets *[]string `json:"sets"`
}

// normalizeExcludeSets trims, lowercases, validates and de-duplicates country
// codes, keeping first-occurrence order. Unknown codes (e.g. "xx") pass here;
// lib/tproxy.sh drops them at apply time with a WARN in the VPN Director log.
// Junk already stored in previous (hand-edited configs) is skipped rather than
// rejecting the whole replace — otherwise deleting a valid country fails with
// 400 because the leftover invalid entry is sent back.
func normalizeExcludeSets(sets, previous []string) ([]string, error) {
	stored := make(map[string]bool, len(previous))
	for _, p := range previous {
		stored[p] = true
		stored[strings.ToLower(strings.TrimSpace(p))] = true
	}
	out := make([]string, 0, len(sets))
	seen := make(map[string]bool, len(sets))
	for _, raw := range sets {
		code := strings.ToLower(strings.TrimSpace(raw))
		if !countryCodeRe.MatchString(code) {
			if stored[raw] || stored[strings.TrimSpace(raw)] || stored[code] {
				continue
			}
			return nil, fmt.Errorf("invalid country code: %q", raw)
		}
		if seen[code] {
			continue
		}
		seen[code] = true
		out = append(out, code)
	}
	return out, nil
}

// handleUpdateExcludeSets returns a handler that replaces the exclusion sets
// list and applies the configuration.
func handleUpdateExcludeSets(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updateExcludeSetsRequest
		if err := decodeJSON(r, &req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if req.Sets == nil {
			jsonError(w, http.StatusBadRequest, "sets field is required")
			return
		}

		unlock, ok := lockLongOp(w, r, deps, applyDeadline)
		if !ok {
			return
		}
		defer unlock()

		writeSaveApplyResult(w, updateAndApply(deps, func(cfg *vpnconfig.VPNDirectorConfig) error {
			sets, err := normalizeExcludeSets(*req.Sets, cfg.Xray.ExcludeSets)
			if err != nil {
				return &httpError{status: http.StatusBadRequest, msg: err.Error()}
			}
			cfg.Xray.ExcludeSets = sets
			return nil
		}))
	}
}

// handleListExcludeIPs returns a handler that lists configured exclusion IPs.
func handleListExcludeIPs(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to load configuration")
			return
		}
		jsonOK(w, map[string]interface{}{"ips": cfg.Xray.ExcludeIPs})
	}
}

// addExcludeIPRequest is the expected JSON body for POST /api/excludes/ips.
type addExcludeIPRequest struct {
	IP string `json:"ip"`
}

// handleAddExcludeIP returns a handler that adds an IPv4/CIDR to the exclusion
// list and applies the configuration.
func handleAddExcludeIP(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req addExcludeIPRequest
		if err := decodeJSON(r, &req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if req.IP == "" {
			jsonError(w, http.StatusBadRequest, "ip is required")
			return
		}
		ip, err := vpnconfig.NormalizeClientAddr(req.IP)
		if err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}

		unlock, ok := lockLongOp(w, r, deps, applyDeadline)
		if !ok {
			return
		}
		defer unlock()

		writeSaveApplyResult(w, updateAndApply(deps, func(cfg *vpnconfig.VPNDirectorConfig) error {
			if !containsAddr(cfg.Xray.ExcludeIPs, ip) {
				cfg.Xray.ExcludeIPs = append(cfg.Xray.ExcludeIPs, ip)
			}
			return nil
		}))
	}
}

// handleDeleteExcludeIP returns a handler that removes an IPv4/CIDR from the
// exclusion list (any stored spelling) and applies the configuration.
func handleDeleteExcludeIP(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip, ok := clientAddrFromQuery(w, r)
		if !ok {
			return
		}

		unlock, ok := lockLongOp(w, r, deps, applyDeadline)
		if !ok {
			return
		}
		defer unlock()

		writeSaveApplyResult(w, updateAndApply(deps, func(cfg *vpnconfig.VPNDirectorConfig) error {
			cfg.Xray.ExcludeIPs = removeAddr(cfg.Xray.ExcludeIPs, ip)
			return nil
		}))
	}
}
