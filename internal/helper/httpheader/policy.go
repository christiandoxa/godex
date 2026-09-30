package httpheader

import (
	"net/http"
	"strings"
)

func ConnectionTokens(headers http.Header) map[string]bool {
	tokens := make(map[string]bool)
	for _, value := range headers.Values("Connection") {
		for _, token := range strings.Split(value, ",") {
			canonical := http.CanonicalHeaderKey(strings.TrimSpace(token))
			if canonical != "" {
				tokens[canonical] = true
			}
		}
	}
	return tokens
}

func IsRequestTransport(name string) bool {
	switch name {
	case "connection", "content-length", "host", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

func IsHop(key string) bool {
	switch strings.ToLower(key) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}
