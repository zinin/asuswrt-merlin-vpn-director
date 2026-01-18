package bot

import "strings"

func isAuthorized(username string, allowedUsers []string) bool {
	if username == "" {
		return false
	}
	for _, allowed := range allowedUsers {
		if strings.EqualFold(allowed, username) {
			return true
		}
	}
	return false
}
