//go:build linux

package superexpose

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"strings"
)

func headerContainsToken(value, token string) bool {
	for _, item := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(item), token) {
			return true
		}
	}
	return false
}

func websocketAccept(key string) string {
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func jsonIDMatches(value any, expected uint64) bool {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Int64()
		return err == nil && parsed >= 0 && uint64(parsed) == expected
	case float64:
		return typed >= 0 && typed == float64(expected)
	default:
		return false
	}
}

func nextAppServerRequestID(value *uint64) uint64 {
	current := *value
	if *value != ^uint64(0) {
		(*value)++
	}
	return current
}
