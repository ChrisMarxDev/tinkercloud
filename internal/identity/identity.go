package identity

import (
	"fmt"
	"strings"
)

type Identity struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// Normalize is deliberately conservative: no provider-specific plus/dot rules.
func Normalize(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	at := strings.LastIndexByte(s, '@')
	if at <= 0 || at == len(s)-1 || strings.Count(s, "@") != 1 {
		return "", fmt.Errorf("invalid email")
	}
	local, domain := s[:at], strings.ToLower(s[at+1:])
	if strings.ContainsAny(local+domain, "\r\n\t ") || !strings.Contains(domain, ".") {
		return "", fmt.Errorf("invalid email")
	}
	return local + "@" + domain, nil
}

func Domain(email string) string {
	if i := strings.LastIndexByte(email, '@'); i >= 0 {
		return email[i+1:]
	}
	return ""
}
