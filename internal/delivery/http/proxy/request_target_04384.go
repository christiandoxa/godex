package proxy

import (
	"encoding/json"
	"net/http"
)

// Prodex 0.438.4's compiled runtime_request_target.mojo validates the
// original HTTP request target, before local provider dispatch, model
// catalog resolution or any authenticated upstream call. Do not inspect
// URL.Path after Go's percent-decoding: that discards the exact safety
// boundary for encoded spaces, traversal, separators and non-ASCII bytes.
func validProdexRequestTarget04384(target string) bool {
	if len(target) == 0 || len(target) > 8192 || target[0] != '/' || len(target) > 1 && target[1] == '/' {
		return false
	}
	pathLength := len(target)
	for index := 0; index < len(target); index++ {
		value := target[index]
		if value > 127 || value <= 32 || value == 127 || value == '#' || value == '\\' {
			return false
		}
		if value == '?' {
			pathLength = index
			break
		}
	}
	for index := pathLength; index < len(target); index++ {
		value := target[index]
		if value > 127 || value <= 32 || value == 127 || value == '#' {
			return false
		}
	}
	segmentStart := 0
	for index := 0; index <= pathLength; index++ {
		if index < pathLength && target[index] != '/' {
			continue
		}
		segment := target[segmentStart:index]
		if segment == "." || segment == ".." {
			return false
		}
		if index < pathLength && index > 0 && target[index-1] == '/' {
			return false
		}
		segmentStart = index + 1
	}
	for index := 0; index < len(target); index++ {
		if target[index] != '%' {
			continue
		}
		if index+2 >= len(target) {
			return false
		}
		high, low := prodexRequestTargetHex04384(target[index+1]), prodexRequestTargetHex04384(target[index+2])
		if high < 0 || low < 0 {
			return false
		}
		decoded := byte((high << 4) | low)
		if index < pathLength && (decoded >= 128 || decoded <= 32 || decoded == 127 ||
			decoded == '/' || decoded == '\\' || decoded == '%' || decoded == '?' || decoded == '#' || decoded == '.') {
			return false
		}
		index += 2
	}
	return true
}

func prodexRequestTargetHex04384(value byte) int {
	switch {
	case value >= '0' && value <= '9':
		return int(value - '0')
	case value >= 'A' && value <= 'F':
		return int(value-'A') + 10
	case value >= 'a' && value <= 'f':
		return int(value-'a') + 10
	default:
		return -1
	}
}

func writeProdexInvalidRequestTarget04384(writer http.ResponseWriter) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(http.StatusBadRequest)
	payload, _ := json.Marshal(map[string]any{"error": map[string]string{
		"code": "invalid_request_target", "message": "request target is invalid",
	}})
	_, _ = writer.Write(payload)
}

// Reject malformed raw HTTP targets before broker handling, profile
// admission or authenticated provider work. Do not percent-decode before
// checking the exact request target.
func rejectProdexRequestTarget04384(writer http.ResponseWriter, request *http.Request) bool {
	target := request.RequestURI
	if target == "" && request.URL != nil {
		target = request.URL.RequestURI()
	}
	if validProdexRequestTarget04384(target) {
		return false
	}
	writeProdexInvalidRequestTarget04384(writer)
	return true
}
