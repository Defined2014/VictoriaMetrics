package vmalertutil

import (
	"fmt"
	"strings"

	"github.com/VictoriaMetrics/VictoriaMetrics/lib/auth"
)

// ReplaceMultitenantsWithAuthToken to replace `multitenant` with auth.String()
func ReplaceMultitenantsWithAuthToken(s string, token *auth.Token) string {
	if token == nil {
		return s
	}
	original := s
	var scheme, host, prefix string
	// find scheme
	n := strings.Index(s, "://")
	if n < 0 {
		scheme = "http"
	} else {
		scheme = s[:n]
	}
	s = s[n+3:]
	// find host
	n = strings.IndexByte(s, '/')
	if n < 0 {
		host = s
		s = s[len(s):]
	} else {
		host = s[:n]
		s = s[n+1:]
	}
	// find suffix
	n = strings.IndexByte(s, '/')
	if n < 0 {
		prefix = s
		s = s[len(s):]
	} else {
		prefix = s[:n]
		s = s[n+1:]
	}
	baseURL := fmt.Sprintf("%s://%s/%s", scheme, host, prefix)
	if len(s) == 0 {
		return original
	}

	n = strings.IndexByte(s, '/')
	if n < 0 {
		return original
	}

	if s[:n] != "multitenant" {
		return original
	}

	return baseURL + "/" + token.String() + s[n:]
}
