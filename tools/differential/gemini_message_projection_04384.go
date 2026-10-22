package main

func geminiMessageProjectionSuffixes04384() []string {
	return []string{"projection-control", "projection-string-array", "projection-alias-array", "projection-empty-separators", "projection-object", "projection-empty-message",
		"projection-system-only", "projection-developer", "projection-instructions-only", "projection-null-input", "projection-cache-control", "projection-alias-priority",
		"projection-implicit-default", "projection-implicit-unknown", "projection-empty-object", "projection-whitespace-instructions", "projection-scalar-parts", "projection-prefix-part", "projection-invalid-part", "projection-media-ignored"}
}

func geminiMessageProjectionFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	input := `[{"type":"message","role":"user","content":"same request"}]`
	messages, extra := `[{"content":"same request","role":"user"}]`, ""
	fixture := requestPrimitiveFixture04384{}
	switch suffix {
	case "projection-control":
	case "projection-string-array":
		input = `[{"type":"message","role":"user","content":["same request"]}]`
	case "projection-alias-array":
		input = `[{"type":"message","role":"user","content":[{"input_text":"same request"}]}]`
	case "projection-empty-separators":
		input = `[{"type":"message","role":"user","content":["first",{"text":""},{"output_text":"last"}]}]`
		messages = `[{"content":"first\n\nlast","role":"user"}]`
	case "projection-object":
		input = `[{"type":"message","role":"user","content":{"type":"input_text","text":"same request"}}]`
		messages = `[{"content":"{\"text\":\"same request\",\"type\":\"input_text\"}","role":"user"}]`
	case "projection-empty-message":
		input = `[{"type":"message","role":"user","content":""}]`
		messages = `[{"content":"","role":"user"}]`
	case "projection-system-only":
		input = `[{"type":"message","role":"system","content":"same request"}]`
		messages = `[{"content":"same request","role":"system"}]`
	case "projection-developer":
		input = `[{"type":"message","role":"developer","content":"same request"}]`
		messages = `[{"content":"same request","role":"system"}]`
	case "projection-instructions-only":
		fixture.body = `{"model":"gemini-3.1-pro-preview","instructions":"same request"}`
		messages = `[{"content":"same request","role":"system"}]`
	case "projection-null-input":
		input, extra, fixture.reject = "null", `,"instructions":"same request"`, true
	case "projection-cache-control":
		input, fixture.reject = `[{"type":"message","role":"user","content":[{"text":"same request","cache_control":null}]}]`, true
	case "projection-alias-priority":
		input = `[{"type":"message","role":"user","content":[{"text":7,"input_text":"same request"}]}]`
	case "projection-implicit-default":
		input = `[{"role":7,"content":"same request"}]`
	case "projection-implicit-unknown":
		input = `[{"role":"unknown","content":"same request"}]`
	case "projection-empty-object":
		input, messages = `[{}]`, `[{"content":"","role":"user"}]`
	case "projection-whitespace-instructions":
		extra = `,"instructions":" \t\u2003"`
	case "projection-scalar-parts":
		input = `[{"type":"message","content":[7,null,false,"same request"]}]`
	case "projection-prefix-part":
		input = `[{"type":"message","content":[{"text":"same request","prefix":null}]}]`
	case "projection-invalid-part":
		input, fixture.reject = `[{"type":"message","content":[{"ignored":true}]},{"role":"user","content":"same request"}]`, true
	case "projection-media-ignored":
		input = `[{"type":"message","content":[{"type":"input_image","image_url":"synthetic-image"},{"text":"same request"}]}]`
	default:
		return requestPrimitiveFixture04384{}, false
	}
	if fixture.body == "" {
		fixture.body = `{"model":"gemini-3.1-pro-preview","input":` + input + extra + `}`
	}
	fixture.wireBody = `{"messages":` + messages + `,"model":"gemini-3.1-pro-preview","stream":false}`
	return fixture, true
}
