package gemini

import "testing"

func TestGemini04377MIMEClassificationMatchesOracle(t *testing.T) {
	for _, test := range []struct {
		uri, want string
	}{
		{"https://example.test/photo.PNG?x=.jpg", "image/png"},
		{"https://example.test/no-suffix?x=.png", "application/octet-stream"},
		{"https://example.test/file.MPEG", "audio/mpeg"},
		{"https://example.test/file.MOV#asset=.mp3", "video/quicktime"},
		{"https://example.test/file.unknown", "application/octet-stream"},
	} {
		if got := geminiMIMEForURI(test.uri); got != test.want {
			t.Fatalf("geminiMIMEForURI(%q) = %q, want %q", test.uri, got, test.want)
		}
	}
}

func TestGemini04377DataURLRequiresBase64Segment(t *testing.T) {
	if got := geminiImagePart(map[string]any{"image_url": "data:image/png;BASE64,AAAA"}); got["inlineData"].(map[string]any)["mimeType"] != "image/png" {
		t.Fatalf("case-insensitive base64 metadata was rejected: %#v", got)
	}
	if got := geminiImagePart(map[string]any{"image_url": "data:image/png,AAAA"}); got["inlineData"] != nil {
		t.Fatalf("non-base64 data URL was accepted as inline data: %#v", got)
	}
	if got := geminiImagePart(map[string]any{"image_url": "data:;base64,AAAA"}); got["inlineData"].(map[string]any)["mimeType"] != "application/octet-stream" {
		t.Fatalf("empty MIME did not use fallback: %#v", got)
	}
	if got := geminiImagePart(map[string]any{"image_url": map[string]any{"image_url": "https://example.test/picture.PNG"}}); got["fileData"].(map[string]any)["mimeType"] != "image/png" {
		t.Fatalf("nested image_url alias was not classified: %#v", got)
	}
}

func TestGemini04377ResponseMediaKeepsNarrowDataURLAndPresenceSemantics(t *testing.T) {
	tests := []struct {
		name string
		part map[string]any
		want any
	}{
		{
			name: "lowercase response image",
			part: map[string]any{"text": "data:image/png;base64,AAAA"},
			want: map[string]any{"type": "input_image", "image_url": "data:image/png;base64,AAAA"},
		},
		{
			name: "uppercase response suffix is text",
			part: map[string]any{"text": "data:image/png;BASE64,AAAA"},
		},
		{
			name: "empty inline MIME is explicit",
			part: map[string]any{"inlineData": map[string]any{"mimeType": "", "data": ""}},
			want: map[string]any{"type": "output_text", "text": "Gemini returned inline  media (0 base64 characters)."},
		},
		{
			name: "empty inline image data is retained",
			part: map[string]any{"inlineData": map[string]any{"mimeType": "image/png", "data": ""}},
			want: map[string]any{"type": "input_image", "image_url": "data:image/png;base64,"},
		},
		{
			name: "empty file MIME is explicit",
			part: map[string]any{"fileData": map[string]any{"fileUri": "https://example.test/file.bin", "mimeType": ""}},
			want: map[string]any{"type": "output_text", "text": "Gemini returned  media: https://example.test/file.bin"},
		},
		{
			name: "empty file URI is retained",
			part: map[string]any{"fileData": map[string]any{"fileUri": ""}},
			want: map[string]any{"type": "output_text", "text": "Gemini returned application/octet-stream media: "},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := geminiMediaContentItem(test.part)
			if test.want == nil {
				if got != nil {
					t.Fatalf("response media = %#v, want nil", got)
				}
				return
			}
			if got == nil || got["type"] != test.want.(map[string]any)["type"] || got["image_url"] != test.want.(map[string]any)["image_url"] || got["text"] != test.want.(map[string]any)["text"] {
				t.Fatalf("response media = %#v, want %#v", got, test.want)
			}
		})
	}
}
