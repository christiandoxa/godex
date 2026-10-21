package gemini

import "testing"

// Tagged 0.438.4 Mojo media_data_url_parts uses
// rich_view_matches_literal["base64"](segment, True): bytewise ASCII
// uppercase folding only. Go's strings.EqualFold treats Unicode U+017F
// LONG S as ASCII 's', incorrectly admitting a non-base64 marker.
func TestProdex04384GeminiMediaBase64MarkerASCIIOnly(t *testing.T) {
	for _, tc := range []struct {
		name, uri  string
		wantInline bool
	}{
		{"lowercase", "data:image/png;base64,AAAA", true},
		{"uppercase", "data:image/png;BASE64,AAAA", true},
		{"mixed-ASCII", "data:image/png;BaSe64,AAAA", true},
		{"long-s-confusable", "data:image/png;baſe64,AAAA", false},
		{"long-s-uppercase", "data:image/png;BAſE64,AAAA", false},
		{"unicode-segment-before-valid", "data:image/png;baſe64;BASE64,AAAA", true},
		{"valid-before-unicode", "data:image/png;base64;baſe64,AAAA", true},
		{"no-base64", "data:image/png,AAAA", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mime, data, valid := geminiDataURL(tc.uri)
			if valid != tc.wantInline || (valid && (mime != "image/png" || data != "AAAA")) {
				t.Fatalf("Gemini data URL %q inline=%t mime=%q data=%q wantInline=%t", tc.uri, valid, mime, data, tc.wantInline)
			}
			item := geminiImagePart(map[string]any{"image_url": tc.uri})
			_, inline := item["inlineData"]
			if inline != tc.wantInline {
				t.Fatalf("Gemini %q emitted incorrect inline media: %#v", tc.uri, item)
			}
		})
	}
}

// The same Mojo media_mime_kind compares ASCII-folded extension bytes.
// Go strings.ToLower maps U+0130 LATIN CAPITAL I WITH DOT to ASCII i,
// making a non-ASCII extension ".GİF" falsely appear to be ".gif".
func TestProdex04384GeminiMediaURIExtensionUsesASCIIOnlyCaseFold(t *testing.T) {
	for _, tc := range []struct{ name, uri, want string }{
		{"canonical-gif", "https://example.test/photo.gif", "image/gif"},
		{"uppercase-GIF", "https://example.test/photo.GIF", "image/gif"},
		{"dotted-I-confusable", "https://example.test/photo.GİF", "application/octet-stream"},
		{"dotted-I-before-query", "https://example.test/photo.GİF?x=.png", "application/octet-stream"},
		{"canonical-before-query", "https://example.test/photo.GIF?x=.png", "image/gif"},
		{"unicode-dotless-i", "https://example.test/photo.GıF", "application/octet-stream"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := geminiMIMEForURI(tc.uri); got != tc.want {
				t.Fatalf("Gemini URI %q MIME=%q want %q", tc.uri, got, tc.want)
			}
			part := geminiImagePart(map[string]any{"image_url": tc.uri})
			data, ok := part["fileData"].(map[string]any)
			if !ok || data["mimeType"] != tc.want {
				t.Fatalf("Gemini media request %q produced %#v", tc.uri, part)
			}
		})
	}
}
