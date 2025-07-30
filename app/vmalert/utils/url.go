package utils

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/VictoriaMetrics/VictoriaMetrics/lib/auth"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/logger"
)

// ParseURL to parse a string to get the baseURL, suffix.
func ParseURL(s string) (string, string, error) {
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
	logger.Infof("ParseURL baseURL=%s", baseURL)
	// find auth token and suffix
	var accountID, projectID uint32
	suffix := ""
	if len(s) > 0 {
		n = strings.IndexByte(s, '/')
		if n < 0 {
			return "", "", fmt.Errorf("invalid auth token: %s", s)
		}
		atStr := s[:n]
		s := s[n+1:]
		atList := strings.Split(atStr, ":")
		switch len(atList) {
		case 1, 2:
		default:
			return "", "", fmt.Errorf("invalid auth token: %s", atStr)
		}
		for idx, item := range atList {
			if idx == 0 {
				// parse AccountID
				accountIDUint64, err := strconv.ParseUint(item, 10, 0)
				if err != nil {
					return "", "", fmt.Errorf("invalid accoutID: %s", atStr)
				}
				accountID = uint32(accountIDUint64)
			} else {
				// parse projectID
				projectIDUint64, err := strconv.ParseUint(item, 10, 0)
				if err != nil {
					return "", "", fmt.Errorf("invalid projectID: %s", atStr)
				}
				projectID = uint32(projectIDUint64)
			}
		}
		if len(s) > 0 {
			if s[len(s)-1] == '/' {
				s = s[:len(s)-1]
			}
			suffix = s
		}
	}
	at := auth.Token{
		AccountID: accountID,
		ProjectID: projectID,
	}
	logger.Infof("BaseURL = %s, Suffix = %s, Token = [%d:%d]", baseURL, suffix, at.String())

	return baseURL, suffix, nil
}
