package gemini

import "strings"

// geminiDataURL parses request media data URLs. The provider request contract
// accepts a case-insensitive base64 metadata segment.
func geminiDataURL(value string) (string, string, bool) {
	if !strings.HasPrefix(value, "data:") {
		return "", "", false
	}
	metadata, data, found := strings.Cut(strings.TrimPrefix(value, "data:"), ",")
	if !found {
		return "", "", false
	}
	segments := strings.Split(metadata, ";")
	base64 := false
	for _, segment := range segments {
		if geminiASCIIEqualFold04384(segment, "base64") {
			base64 = true
			break
		}
	}
	if !base64 {
		return "", "", false
	}
	mime := segments[0]
	if strings.TrimSpace(mime) == "" {
		mime = "application/octet-stream"
	}
	return mime, data, true
}

// The 0.438.4 Mojo response-media parser compares the decoded data-URL
// metadata `base64` segment with ASCII-only byte folding. Unicode simple
// case folding would let U+017F LONG S impersonate the ASCII `s`.
func geminiASCIIEqualFold04384(input, literal string) bool {
	if len(input) != len(literal) {
		return false
	}
	for i := 0; i < len(input); i++ {
		value := input[i]
		if value >= 'A' && value <= 'Z' {
			value += 'a' - 'A'
		}
		if value != literal[i] {
			return false
		}
	}
	return true
}

// geminiResponseDataURL parses response text media. Prodex keeps this
// response-side contract narrower: only an exact lowercase ;base64 suffix is
// projected as inline media.
func geminiResponseDataURL(value string) (string, string, bool) {
	if !strings.HasPrefix(value, "data:") {
		return "", "", false
	}
	metadata, data, found := strings.Cut(strings.TrimPrefix(value, "data:"), ",")
	if !found || len(metadata) < len(";base64") || !strings.HasSuffix(metadata, ";base64") {
		return "", "", false
	}
	return strings.TrimSuffix(metadata, ";base64"), data, true
}

func geminiASCIILower04384(value string) string {
	bytes := []byte(value)
	for at := range bytes {
		if bytes[at] >= 'A' && bytes[at] <= 'Z' {
			bytes[at] += 'a' - 'A'
		}
	}
	return string(bytes)
}

func geminiMIMEForURI(uri string) string {
	if cutoff := strings.IndexAny(uri, "?#"); cutoff >= 0 {
		uri = uri[:cutoff]
	}
	// The tagged Mojo media_mime_kind folds A-Z bytes only. Unicode
	// casing changes the meaning of URI extensions (for example U+0130
	// dotted capital I would collapse `.GİF` into a false `.gif`).
	lower := geminiASCIILower04384(uri)
	switch {
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(lower, ".webp"):
		return "image/webp"
	case strings.HasSuffix(lower, ".gif"):
		return "image/gif"
	case strings.HasSuffix(lower, ".pdf"):
		return "application/pdf"
	case strings.HasSuffix(lower, ".mp3"), strings.HasSuffix(lower, ".mpeg"):
		return "audio/mpeg"
	case strings.HasSuffix(lower, ".wav"):
		return "audio/wav"
	case strings.HasSuffix(lower, ".mp4"):
		return "video/mp4"
	case strings.HasSuffix(lower, ".mov"):
		return "video/quicktime"
	default:
		return "application/octet-stream"
	}
}
