package openai

import "strings"

const backendAPIPath = "/backend-api"

func upstreamPath(basePath, requestPath string) string {
	requestPath = normalizeOpenAIPath(requestPath)
	basePath = strings.TrimRight(basePath, "/")
	if basePath == "" {
		return "/" + strings.TrimLeft(requestPath, "/")
	}
	if basePath == backendAPIPath && strings.HasPrefix(requestPath, backendAPIPath) &&
		(requestPath == backendAPIPath || strings.HasPrefix(requestPath, backendAPIPath+"/")) {
		return basePath + strings.TrimPrefix(requestPath, backendAPIPath)
	}
	return basePath + "/" + strings.TrimLeft(requestPath, "/")
}

func normalizeOpenAIPath(requestPath string) string {
	const upstreamPath = backendAPIPath + "/codex"
	for _, mountPath := range []string{backendAPIPath + "/godex", backendAPIPath + "/prodex"} {
		if suffix, ok := strings.CutPrefix(requestPath, mountPath+"/v"); ok {
			if slash := strings.IndexByte(suffix, '/'); slash > 0 && legacyVersionSegment(suffix[:slash]) {
				return upstreamPath + suffix[slash:]
			}
		}
		if suffix, ok := strings.CutPrefix(requestPath, mountPath); ok && (suffix == "" || strings.HasPrefix(suffix, "/")) {
			return upstreamPath + suffix
		}
	}
	return requestPath
}

func legacyVersionSegment(segment string) bool {
	if segment == "" {
		return false
	}
	digit := false
	for _, character := range segment {
		if character >= '0' && character <= '9' {
			digit = true
			continue
		}
		if character != '.' {
			return false
		}
	}
	return digit
}
