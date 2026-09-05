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
// codes, keeping first-occurrence order. Unknown codes (e.g. "xx") pass here
// and fail at apply time in lib/ipset.sh, which the auto-apply surfaces.
func normalizeExcludeSets(sets []string) ([]string, error) {
	out := make([]string, 0, len(sets))
	seen := make(map[string]bool, len(sets))
	for _, raw := range sets {
		code := strings.ToLower(strings.TrimSpace(raw))
		if !countryCodeRe.MatchString(code) {
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
		unlock := lockLongOp(w, deps, applyDeadline)
		defer unlock()

		var req updateExcludeSetsRequest
		if err := decodeJSON(r, &req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if req.Sets == nil {
			jsonError(w, http.StatusBadRequest, "sets field is required")
			return
		}

		sets, err := normalizeExcludeSets(*req.Sets)
		if err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}

		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to load configuration")
			return
		}

		cfg.Xray.ExcludeSets = sets

		writeSaveApplyResult(w, saveAndApply(deps, cfg))
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
		unlock := lockLongOp(w, deps, applyDeadline)
		defer unlock()

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

		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to load configuration")
			return
		}

		if !containsAddr(cfg.Xray.ExcludeIPs, ip) {
			cfg.Xray.ExcludeIPs = append(cfg.Xray.ExcludeIPs, ip)
		}

		writeSaveApplyResult(w, saveAndApply(deps, cfg))
	}
}

// handleDeleteExcludeIP returns a handler that removes an IPv4/CIDR from the
// exclusion list (any stored spelling) and applies the configuration.
func handleDeleteExcludeIP(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		unlock := lockLongOp(w, deps, applyDeadline)
		defer unlock()

		ip, ok := clientAddrFromQuery(w, r)
		if !ok {
			return
		}

		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to load configuration")
			return
		}

		cfg.Xray.ExcludeIPs = removeAddr(cfg.Xray.ExcludeIPs, ip)

		writeSaveApplyResult(w, saveAndApply(deps, cfg))
	}
}
