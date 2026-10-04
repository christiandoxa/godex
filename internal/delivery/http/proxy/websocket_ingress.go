package proxy

import (
	"crypto/sha1"
	"encoding/base64"
	"net/http"
	"strings"
)

func isWebSocketUpgradeRequest(request *http.Request) bool {
	if request == nil {
		return false
	}
	for _, value := range request.Header.Values("Upgrade") {
		if strings.EqualFold(strings.TrimSpace(value), "websocket") {
			return true
		}
	}
	return false
}

func websocketRequestError(request *http.Request) (int, string) {
	if !supportedWebSocketPath(request.URL.Path) {
		return http.StatusNotFound, "websocket path is not supported"
	}
	if websocketRequestKey(request) != "" {
		return 0, ""
	}
	return http.StatusBadRequest, "Missing Sec-WebSocket-Key header for runtime auto-rotate websocket proxy."
}

func websocketRequestKey(request *http.Request) string {
	if request == nil {
		return ""
	}
	for _, key := range request.Header.Values("Sec-WebSocket-Key") {
		if key = strings.TrimSpace(key); key != "" {
			return key
		}
	}
	return ""
}

func websocketAcceptKey(key string) string {
	digest := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(digest[:])
}

func supportedWebSocketPath(path string) bool {
	suffixStart := websocketMountSuffixStart(path)
	effective := path
	if suffixStart >= 0 {
		effective = path[suffixStart:]
	}
	responses := false
	if suffixStart >= 0 {
		responses = effective == "/responses" || strings.HasSuffix(effective, "/codex/responses")
	} else {
		responses = strings.HasSuffix(path, "/codex/responses")
	}
	if responses || strings.HasSuffix(effective, "/realtime") || strings.HasSuffix(effective, "/live") {
		return true
	}
	return websocketEffectiveLiveCall(effective)
}

func websocketUsesMessageRouting(path string) bool {
	effective := websocketEffectivePath(path)
	return effective == "/responses" ||
		strings.HasSuffix(effective, "/codex/responses") ||
		websocketRealtimeDuplexPath(path)
}

func websocketRealtimeDuplexPath(path string) bool {
	effective := websocketEffectivePath(path)
	return strings.HasSuffix(effective, "/realtime") ||
		strings.HasSuffix(effective, "/live") ||
		websocketEffectiveLiveCall(effective)
}

func websocketEffectivePath(path string) string {
	if suffixStart := websocketMountSuffixStart(path); suffixStart >= 0 {
		return path[suffixStart:]
	}
	return path
}

func websocketMountSuffixStart(path string) int {
	const legacy = "/backend-api/prodex/v"
	const mount = "/backend-api/prodex"
	if strings.HasPrefix(path, legacy) {
		versionStart := len(legacy)
		if slashOffset := strings.IndexByte(path[versionStart:], '/'); slashOffset >= 0 {
			slash := versionStart + slashOffset
			if websocketLegacyVersionSegment(path[versionStart:slash]) {
				return slash
			}
		}
	}
	if strings.HasPrefix(path, mount) && (len(path) == len(mount) || len(path) > len(mount) && path[len(mount)] == '/') {
		return len(mount)
	}
	return -1
}

func websocketLegacyVersionSegment(segment string) bool {
	hasDigit := false
	for _, character := range segment {
		switch {
		case character >= '0' && character <= '9':
			hasDigit = true
		case character != '.':
			return false
		}
	}
	return hasDigit
}

func websocketEffectiveLiveCall(path string) bool {
	const marker = "/live/"
	for start := 0; ; {
		index := strings.Index(path[start:], marker)
		if index < 0 {
			return false
		}
		call := path[start+index+len(marker):]
		if call != "" && !strings.Contains(call, "/") {
			return true
		}
		start += index + len(marker)
		if start >= len(path) {
			return false
		}
	}
}
