package main

import "errors"

func geminiSourceIDFixture04384(scenario string) (member, expected string, ok bool) {
	switch scenario {
	case "buffered-source-id-string", "buffered-source-id-wrapped":
		return `"id":"source-id",`, "native-preserved", true
	case "buffered-source-id-empty":
		return `"id":"",`, "native-preserved", true
	case "buffered-source-id-null":
		return `"id":null,`, "", true
	}
	return "", "", false
}

func geminiSourceIDResponse04384(scenario string) (string, bool) {
	member, _, ok := geminiSourceIDFixture04384(scenario)
	if !ok {
		return "", false
	}
	response := `{` + member + `"responseId":"native-preserved","candidates":[{"content":{"parts":[{"text":"synthetic-ok"}]},"finishReason":"STOP"}],"modelVersion":"gemini-3.1-pro-preview","usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1,"totalTokenCount":3}}`
	if scenario == "buffered-source-id-wrapped" {
		response = `{` + member + `"response":` + response + `}`
	}
	return response, true
}

func geminiSourceIDInvariant04384(scenario string, body map[string]any) error {
	_, expected, ok := geminiSourceIDFixture04384(scenario)
	if !ok {
		return errors.New("unknown Gemini source identity fixture")
	}
	if expected == "" {
		return geminiBufferedIdentityInvariant043711(body)
	}
	if body["id"] != expected {
		return errors.New("Gemini string source id triggered an unneeded identity replacement")
	}
	if _, present := body["created_at"]; present {
		return errors.New("Gemini buffered response exposed a native timestamp")
	}
	return nil
}
