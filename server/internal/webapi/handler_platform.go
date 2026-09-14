package webapi

import "net/http"

// handlePlatform returns a handler that reports the platform facts the
// Clients tab builds its route list from: `vpn-director.sh platform`,
// decoded. 503 when the script cannot answer - the page then offers xray
// and the routes its clients already use.
func handlePlatform(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		extendWriteDeadline(w, statusDeadline)

		info, err := deps.VPN.Platform()
		if err != nil {
			jsonError(w, http.StatusServiceUnavailable, "platform info unavailable")
			return
		}
		jsonOK(w, info)
	}
}
