package gemini

import "testing"

func TestProdex04377GeminiImageMimeUsesPathSuffix(t *testing.T) {
	for _, test := range []struct {
		name string
		uri  string
		want string
	}{
		{name: "query does not change suffix", uri: "https://example.test/photo.PNG?x=.jpg", want: "image/png"},
		{name: "fragment does not change suffix", uri: "https://example.test/video.MOV#asset=.mp3", want: "video/quicktime"},
		{name: "query without path suffix is unknown", uri: "https://example.test/file?asset=.png", want: "application/octet-stream"},
		{name: "pdf", uri: "https://example.test/report.pdf", want: "application/pdf"},
		{name: "unknown", uri: "https://example.test/file.unknown", want: "application/octet-stream"},
		{name: "encoded suffix stays encoded", uri: "https://example.test/photo%2Epng", want: "application/octet-stream"},
		{name: "raw URI extension", uri: "https://example.png", want: "image/png"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := geminiImageMime(test.uri); got != test.want {
				t.Fatalf("geminiImageMime(%q) = %q, want %q", test.uri, got, test.want)
			}
		})
	}
}

func TestProdex04377GeminiDataURLRequiresBase64(t *testing.T) {
	part := geminiImagePart(map[string]any{"image_url": "data:image/png,AAAA"})
	file, ok := part["fileData"].(map[string]any)
	if !ok || file["fileUri"] != "data:image/png,AAAA" || file["mimeType"] != "application/octet-stream" {
		t.Fatalf("non-base64 data URL = %#v", part)
	}

	part = geminiImagePart(map[string]any{"image_url": "data:image/png;BASE64,AAAA"})
	inline, ok := part["inlineData"].(map[string]any)
	if !ok || inline["mimeType"] != "image/png" || inline["data"] != "AAAA" {
		t.Fatalf("base64 data URL = %#v", part)
	}

	part = geminiImagePart(map[string]any{"image_url": "data:;base64,AAAA"})
	inline, ok = part["inlineData"].(map[string]any)
	if !ok || inline["mimeType"] != "application/octet-stream" {
		t.Fatalf("default data URL MIME = %#v", part)
	}
}

func TestProdex04377GeminiResponseDataURLRequiresLowercaseBase64(t *testing.T) {
	if got := geminiMediaContentItem(map[string]any{"text": "data:image/png;BASE64,AAAA"}); got != nil {
		t.Fatalf("uppercase response data URL accepted: %#v", got)
	}
	got := geminiMediaContentItem(map[string]any{"text": "data:image/png;base64,AAAA"})
	if got == nil || got["type"] != "input_image" || got["image_url"] != "data:image/png;base64,AAAA" {
		t.Fatalf("response data URL = %#v", got)
	}
	if got := geminiMediaContentItem(map[string]any{"text": "data:image/png,AAAA"}); got != nil {
		t.Fatalf("non-base64 response data URL accepted: %#v", got)
	}
}
