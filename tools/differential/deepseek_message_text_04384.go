package main

import "strings"

func deepSeekMessageTextSuffixes04384() []string {
	return []string{"message-text-simple-alias", "message-text-normal-alias", "message-text-simple-untagged", "message-text-simple-empty", "message-text-normal-empty", "message-text-simple-blank", "message-text-normal-blank", "message-text-simple-alias-empty", "message-text-normal-alias-empty", "message-text-simple-content-priority", "message-text-normal-visible", "message-text-normal-only-empty", "message-text-normal-only-alias", "message-text-simple-only-empty", "message-text-simple-only-alias"}
}

func deepSeekMessageTextFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	switch suffix {
	case "message-text-normal-only-empty", "message-text-normal-only-alias", "message-text-simple-only-empty", "message-text-simple-only-alias":
		role, fields, text := "user", `"content":""`, `""`
		if strings.Contains(suffix, "-normal-") {
			role = "developer"
		}
		if strings.HasSuffix(suffix, "-alias") {
			fields = `"text":"alias text"`
			if role == "user" {
				text = `"alias text"`
			}
		}
		body := `{"model":"gemini-3.1-pro-preview","input":[{"type":"message","role":"` + role + `",` + fields + `}]}`
		wire := `{"messages":[{"content":` + text + `,"role":"user"}],"model":"gemini-3.1-pro-preview","stream":false}`
		return requestPrimitiveFixture04384{body: body, wireBody: wire}, true
	}
	fields, text := `"type":"message","role":"user","text":"alias text"`, `"alias text"`
	blank := false
	switch suffix {
	case "message-text-simple-alias", "message-text-normal-alias":
	case "message-text-simple-untagged":
		fields = `"role":"user","text":"alias text"`
	case "message-text-simple-empty", "message-text-normal-empty":
		fields, text, blank = `"type":"message","role":"user","content":""`, `""`, true
	case "message-text-simple-blank", "message-text-normal-blank":
		fields, text, blank = `"type":"message","role":"user","content":" \t "`, `" \t "`, true
	case "message-text-simple-alias-empty", "message-text-normal-alias-empty":
		fields, text, blank = `"type":"message","role":"user","text":""`, `""`, true
	case "message-text-simple-content-priority", "message-text-normal-visible":
		fields, text = `"type":"message","role":"user","content":"visible","text":"ignored"`, `"visible"`
	default:
		return requestPrimitiveFixture04384{}, false
	}
	inputPrefix, messagesPrefix := "", ""
	normal := strings.Contains(suffix, "-normal-")
	if normal {
		inputPrefix = `{"type":"message","role":"developer","content":"synthetic instruction"},`
		messagesPrefix = `{"content":"synthetic instruction","role":"system"},`
	}
	message := `{"content":` + text + `,"role":"user"},`
	if normal && (blank || suffix == "message-text-normal-alias") {
		message = ""
	}
	body := `{"model":"gemini-3.1-pro-preview","input":[` + inputPrefix + `{` + fields + `},{"type":"message","role":"user","content":"same request"}]}`
	wire := `{"messages":[` + messagesPrefix + message + `{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stream":false}`
	return requestPrimitiveFixture04384{body: body, wireBody: wire}, true
}
