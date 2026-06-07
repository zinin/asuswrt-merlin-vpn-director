#!/usr/bin/env bash
###############################################################################
# lib/xrayconf.sh - Build Xray proxy-out outbound + config.json from a server
# JSON object (as stored in servers.json). Pure jq transforms; no side effects
# on source. Used by configure.sh; unit-tested via bats.
# Mirrors the Go generator in server/internal/service/xray.go — keep both in sync.
###############################################################################

# xrayconf_build_outbound: reads one server JSON object on stdin,
# prints the proxy-out outbound JSON object on stdout. Fails (rc=1) on an
# unsupported network (non-tcp) or security (not tls/reality) instead of
# emitting a silently-broken outbound. Self-contained: pure jq; errors to stderr.
xrayconf_build_outbound() {
    local server_json net sec
    server_json="$(cat)"
    if ! printf '%s' "$server_json" | jq -e 'type == "object" and (.address // "") != "" and (.uuid // "") != ""' >/dev/null 2>&1; then
        printf 'xrayconf: invalid/empty server JSON (need object with address and uuid)\n' >&2
        return 1
    fi
    net="$(printf '%s' "$server_json" | jq -r '.network // ""')"
    sec="$(printf '%s' "$server_json" | jq -r '.security // ""')"
    case "$net" in ""|tcp) ;; *) printf 'xrayconf: unsupported network "%s" (only tcp)\n' "$net" >&2; return 1 ;; esac
    case "$sec" in ""|tls|reality) ;; *) printf 'xrayconf: unsupported security "%s" (only tls/reality)\n' "$sec" >&2; return 1 ;; esac
    printf '%s' "$server_json" | jq '
      def trimempty: with_entries(select(.value != null and .value != "" and .value != []));
      ((.network // "") | if . == "" then "tcp" else . end) as $net
      | (.security // "") as $sec
      | {
          protocol: "vless",
          settings: { vnext: [ {
            address: .address,
            port: .port,
            users: [ ( { id: .uuid, encryption: "none" }
                       + (if (.flow // "") != "" and (.security // "") != "" then { flow: .flow } else {} end) ) ]
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
