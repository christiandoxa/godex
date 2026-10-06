package proxy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
)

type smartContextSource struct {
	inputIndex int
	digest     string
	byteLen    int
}

func smartContextInlineReference(source smartContextSource) string {
	return "[godex-context-ref v=1 source=original-input[" +
		strconv.Itoa(source.inputIndex) + "] digest=" + source.digest +
		" bytes=" + strconv.Itoa(source.byteLen) + "]"
}

func smartContextArtifactID(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "sc2:" + hex.EncodeToString(sum[:])
}

func smartContextExpandInlineReferences(original, candidate any) (any, bool) {
	originalObject, ok := original.(map[string]any)
	if !ok {
		return nil, false
	}
	originalInput, ok := originalObject["input"].([]any)
	if !ok {
		return nil, false
	}
	expanded := deepCloneJSON(candidate)
	if !smartContextExpandValue(expanded, originalInput) {
		return nil, false
	}
	return expanded, true
}

func smartContextExpandValue(value any, originalInput []any) bool {
	switch typed := value.(type) {
	case []any:
		for index, item := range typed {
			if text, ok := item.(string); ok && strings.HasPrefix(text, "[godex-context-ref ") {
				source, ok := smartContextResolveInlineReference(text, originalInput)
				if !ok {
					return false
				}
				typed[index] = source
				continue
			}
			if !smartContextExpandValue(item, originalInput) {
				return false
			}
		}
	case map[string]any:
		for key, item := range typed {
			if text, ok := item.(string); ok && strings.HasPrefix(text, "[godex-context-ref ") {
				source, ok := smartContextResolveInlineReference(text, originalInput)
				if !ok {
					return false
				}
				typed[key] = source
				continue
			}
			if !smartContextExpandValue(item, originalInput) {
				return false
			}
		}
	}
	return true
}

func smartContextResolveInlineReference(reference string, originalInput []any) (string, bool) {
	const prefix = "[godex-context-ref v=1 source=original-input["
	body, ok := strings.CutPrefix(reference, prefix)
	if !ok || !strings.HasSuffix(body, "]") {
		return "", false
	}
	body = strings.TrimSuffix(body, "]")
	indexText, rest, ok := strings.Cut(body, "] digest=")
	if !ok {
		return "", false
	}
	digest, byteText, ok := strings.Cut(rest, " bytes=")
	if !ok {
		return "", false
	}
	index, err := strconv.Atoi(indexText)
	if err != nil || index < 0 || index >= len(originalInput) {
		return "", false
	}
	byteLen, err := strconv.Atoi(byteText)
	if err != nil || byteLen < 0 || !smartContextArtifactIDValid(digest) {
		return "", false
	}
	return smartContextFindText(originalInput[index], digest, byteLen)
}

func smartContextFindText(value any, digest string, byteLen int) (string, bool) {
	switch typed := value.(type) {
	case string:
		if len(typed) == byteLen && smartContextArtifactID(typed) == digest {
			return typed, true
		}
	case []any:
		for _, item := range typed {
			if found, ok := smartContextFindText(item, digest, byteLen); ok {
				return found, true
			}
		}
	case map[string]any:
		for _, item := range typed {
			if found, ok := smartContextFindText(item, digest, byteLen); ok {
				return found, true
			}
		}
	}
	return "", false
}

func smartContextArtifactIDValid(value string) bool {
	if len(value) != 68 || !strings.HasPrefix(value, "sc2:") {
		return false
	}
	for _, current := range value[4:] {
		if (current < '0' || current > '9') && (current < 'a' || current > 'f') {
			return false
		}
	}
	return true
}

func smartContextRoundTripExact(original, expanded any) bool {
	object, ok := expanded.(map[string]any)
	if !ok {
		return false
	}
	input, ok := object["input"].([]any)
	if !ok || len(input) == 0 {
		return false
	}
	last, ok := input[len(input)-1].(map[string]any)
	if !ok || last["type"] != "message" || last["role"] != "developer" ||
		last["content"] != smartContextInlineReferenceProtocol {
		return false
	}
	object["input"] = input[:len(input)-1]
	originalBytes, err := json.Marshal(original)
	if err != nil {
		return false
	}
	expandedBytes, err := json.Marshal(expanded)
	if err != nil {
		return false
	}
	return bytes.Equal(originalBytes, expandedBytes)
}
