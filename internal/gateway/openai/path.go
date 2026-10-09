package openai

import (
	"net/url"
	"strings"
)

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

func protectDotSegments(path, rawPath string) (string, string) {
	if rawPath == "" {
		rawPath = (&url.URL{Path: path}).EscapedPath()
	}
	protected := escapeDotSegments(rawPath)
	decoded, err := url.PathUnescape(protected)
	if err != nil {
		return path, rawPath
	}
	return decoded, protected
}

func escapeDotSegments(path string) string {
	segments := strings.Split(path, "/")
	changed := false
	for index, segment := range segments {
		switch encodedDotSegmentLen(segment) {
		case 1:
			segments[index], changed = "%252e", true
		case 2:
			segments[index], changed = "%252e%252e", true
		}
	}
	if !changed {
		return path
	}
	return strings.Join(segments, "/")
}

func encodedDotSegmentLen(segment string) int {
	bytes := []byte(segment)
	dots := 0
	for index := 0; index < len(bytes); {
		switch {
		case bytes[index] == '.':
			dots++
			index++
		case index+2 < len(bytes) && bytes[index] == '%' && bytes[index+1] == '2' &&
			(bytes[index+2] == 'e' || bytes[index+2] == 'E'):
			dots++
			index += 3
		default:
			return 0
		}
	}
	if dots == 1 || dots == 2 {
		return dots
	}
	return 0
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
