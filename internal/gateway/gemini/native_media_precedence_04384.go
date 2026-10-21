package gemini

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Mojo media_field selects the first PRESENT JSON key, regardless of value
// type. The regular firstPresentNative helper loses that presence bit.
func firstPresentNativeWithPresence04384(object map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if value, found := object[key]; found {
			return value, true
		}
	}
	return nil, false
}

func geminiSpecialPartText(part map[string]any) string {
	if raw, present := part["executableCode"]; present {
		code, _ := raw.(map[string]any)
		language := nativeString(code["language"])
		if language == "" {
			language = "text"
		}
		body := nativeString(code["code"])
		if strings.TrimSpace(body) == "" {
			return ""
		}
		return fmt.Sprintf("Gemini executable code (%s):\n```%s\n%s\n```", language, language, body)
	}
	if raw, present := part["codeExecutionResult"]; present {
		result, _ := raw.(map[string]any)
		outcome := nativeString(result["outcome"])
		if outcome == "" {
			outcome = "OUTCOME_UNSPECIFIED"
		}
		return fmt.Sprintf("Gemini code execution result (%s):\n```text\n%s\n```", outcome, nativeString(result["output"]))
	}
	if video, ok := part["videoMetadata"]; ok {
		encoded, _ := json.Marshal(video)
		return "Gemini video metadata: " + string(encoded)
	}
	return ""
}

func geminiMediaContentItem(part map[string]any) map[string]any {
	inlineValue, present := firstPresentNativeWithPresence04384(part, "inlineData", "inline_data")
	if present {
		inline, _ := inlineValue.(map[string]any)
		if inline == nil {
			return nil
		}
		mimeValue := firstPresentNative(inline, "mimeType", "mime_type")
		mime, mimeIsString := mimeValue.(string)
		if !mimeIsString {
			mime = "application/octet-stream"
		}
		data, dataIsString := inline["data"].(string)
		if !dataIsString {
			return nil
		}
		if strings.HasPrefix(mime, "image/") {
			return map[string]any{"type": "input_image", "image_url": "data:" + mime + ";base64," + data}
		}
		return map[string]any{"type": "output_text", "text": fmt.Sprintf("Gemini returned inline %s media (%d base64 characters).", mime, len(data))}
	}
	fileValue, present := firstPresentNativeWithPresence04384(part, "fileData", "file_data")
	if present {
		file, _ := fileValue.(map[string]any)
		if file == nil {
			return nil
		}
		uri, uriIsString := firstPresentNative(file, "fileUri", "file_uri").(string)
		if !uriIsString {
			return nil
		}
		mimeValue := firstPresentNative(file, "mimeType", "mime_type")
		mime, mimeIsString := mimeValue.(string)
		if !mimeIsString {
			mime = geminiMIMEForURI(uri)
		}
		if strings.HasPrefix(mime, "image/") {
			return map[string]any{"type": "input_image", "image_url": uri}
		}
		return map[string]any{"type": "output_text", "text": "Gemini returned " + mime + " media: " + uri}
	}
	if text, _ := part["text"].(string); strings.HasPrefix(text, "data:") {
		mime, body, ok := geminiResponseDataURL(text)
		if !ok {
			return nil
		}
		if strings.HasPrefix(mime, "image/") {
			return map[string]any{"type": "input_image", "image_url": "data:" + mime + ";base64," + body}
		}
		return map[string]any{"type": "output_text", "text": fmt.Sprintf("Gemini returned inline %s media (%d base64 characters).", mime, len(body))}
	}
	return nil
}
