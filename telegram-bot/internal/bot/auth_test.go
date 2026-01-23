// internal/bot/auth_test.go
package bot

import "testing"

func TestAuth_IsAuthorized(t *testing.T) {
	tests := []struct {
		name         string
		username     string
		allowedUsers []string
		want         bool
	}{
		{
			name:         "exact match",
			username:     "alice",
			allowedUsers: []string{"alice", "bob"},
			want:         true,
		},
		{
			name:         "case insensitive match lowercase input",
			username:     "alice",
			allowedUsers: []string{"Alice", "Bob"},
			want:         true,
		},
		{
			name:         "case insensitive match uppercase input",
			username:     "ALICE",
			allowedUsers: []string{"alice", "bob"},
			want:         true,
		},
		{
			name:         "case insensitive match mixed case",
			username:     "AlIcE",
			allowedUsers: []string{"aLiCe", "bob"},
			want:         true,
		},
		{
			name:         "not in list",
			username:     "charlie",
			allowedUsers: []string{"alice", "bob"},
			want:         false,
		},
		{
			name:         "empty username",
			username:     "",
			allowedUsers: []string{"alice", "bob"},
			want:         false,
		},
		{
			name:         "empty allowed list",
			username:     "alice",
			allowedUsers: []string{},
			want:         false,
		},
		{
			name:         "nil allowed list",
			username:     "alice",
			allowedUsers: nil,
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := NewAuth(tt.allowedUsers)
			got := auth.IsAuthorized(tt.username)
			if got != tt.want {
				t.Errorf("Auth.IsAuthorized(%q) = %v, want %v", tt.username, got, tt.want)
			}
		})
	}
}

func TestNewAuth_NormalizesToLowercase(t *testing.T) {
	auth := NewAuth([]string{"ALICE", "Bob", "charlie"})

	if !auth.IsAuthorized("alice") {
		t.Error("expected 'alice' to be authorized")
	}
	if !auth.IsAuthorized("bob") {
		t.Error("expected 'bob' to be authorized")
	}
	if !auth.IsAuthorized("CHARLIE") {
		t.Error("expected 'CHARLIE' to be authorized")
	}
}
