package main

import "strings"

func deepSeekMessageContentSuffixes04384() []string {
	return []string{"bridge-content-normal-object", "bridge-content-object-only", "bridge-content-normal-object-empty", "bridge-content-normal-unicode", "bridge-content-normal-parts-empty", "bridge-content-simple-parts-empty", "bridge-content-normal-parts-strings", "bridge-content-simple-text-control", "bridge-content-normal-text-control", "bridge-content-normal-mixed-control", "bridge-content-image-rejected", "bridge-content-cache-rejected"}
}

func deepSeekMessageContentFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	value, content := `{"type":"input_text","text":"alpha"}`, `"{\"text\":\"alpha\",\"type\":\"input_text\"}"`
	reject := false
	switch suffix {
	case "bridge-content-normal-object", "bridge-content-object-only":
	case "bridge-content-normal-object-empty":
		value, content = `{"text":""}`, `"{\"text\":\"\"}"`
	case "bridge-content-normal-unicode":
		text := "<>&/🌍\u2028\u2029"
		value = `{"text":"` + text + `","z":1e0}`
		content = `"{\"text\":\"` + text + `\",\"z\":1.0}"`
	case "bridge-content-normal-parts-empty", "bridge-content-simple-parts-empty":
		value, content = `[{"text":""},{"text":"last"}]`, `"\nlast"`
	case "bridge-content-normal-parts-strings":
		value, content = `["",{"text":""},"last"]`, `"\n\nlast"`
	case "bridge-content-simple-text-control", "bridge-content-normal-text-control":
		value, content = `[{"type":"input_text","text":"alpha"}]`, `"alpha"`
	case "bridge-content-normal-mixed-control":
		value, content = `[null,false,7,{"text":"last"}]`, `"last"`
	case "bridge-content-image-rejected":
		value, reject = `[{"type":"input_image","image_url":"https://example.invalid/synthetic.png"}]`, true
	case "bridge-content-cache-rejected":
		value, reject = `[{"text":"alpha","cache_control":null}]`, true
	default:
		return requestPrimitiveFixture04384{}, false
	}
	inputPrefix, messagesPrefix := "", ""
	if strings.Contains(suffix, "-normal-") {
		inputPrefix = `{"type":"message","role":"developer","content":"synthetic instruction"},`
		messagesPrefix = `{"content":"synthetic instruction","role":"system"},`
	}
	body := `{"model":"gemini-3.1-pro-preview","input":[` + inputPrefix + `{"type":"message","role":"user","content":` + value + `},{"type":"message","role":"user","content":"same request"}]}`
	wire := `{"messages":[` + messagesPrefix + `{"content":` + content + `,"role":"user"},{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stream":false}`
	return requestPrimitiveFixture04384{body: body, wireBody: wire, reject: reject}, true
}
