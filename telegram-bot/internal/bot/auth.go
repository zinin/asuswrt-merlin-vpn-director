// internal/bot/auth.go
package bot

import "strings"

// Auth handles user authorization with O(1) lookup
type Auth struct {
	allowedUsers map[string]bool // lowercase usernames
}

// NewAuth creates a new Auth with case-insensitive username matching.
// Usernames are normalized to lowercase.
func NewAuth(allowedUsers []string) *Auth {
	allowed := make(map[string]bool)
	for _, u := range allowedUsers {
		allowed[strings.ToLower(u)] = true
	}
	return &Auth{allowedUsers: allowed}
}

// IsAuthorized checks if a username is authorized (case-insensitive).
// Empty username is never authorized.
func (a *Auth) IsAuthorized(username string) bool {
	if username == "" {
		return false
	}
	return a.allowedUsers[strings.ToLower(username)]
}
