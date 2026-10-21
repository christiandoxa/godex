package gemini

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiPresentNonObjectEnvelopeHidesOuterFields(t *testing.T) {
	outer := `"responseId":"outer-id","modelVersion":"outer-model","candidates":[{"content":{"parts":[{"text":"outer-must-not-leak"}]}}],"usageMetadata":{"promptTokenCount":99}`
	for _, value := range []string{`null`, `[]`, `[{}]`, `false`, `7`, `"inner"`, `{}`} {
		for _, chat := range []bool{false, true} {
			wire := `{"traceId":"outer-trace",` + outer + `,"response":` + value + `}`
			body, err := geminiEnvelopeResponse04384(wire, chat)
			if err != nil {
				t.Fatalf("value=%s chat=%v: %v", value, chat, err)
			}
			var response map[string]any
			if json.Unmarshal(body, &response) != nil {
				t.Fatalf("invalid response: %s", body)
			}
			failure, _ := response["error"].(map[string]any)
			output, _ := response["output"].([]any)
			if chat {
				usage, usageOK := response["usage"].(map[string]any)
				metadata, metadataOK := response["metadata"].(map[string]any)
				if !usageOK || !metadataOK || len(usage) != 0 || len(metadata) != 0 {
					t.Errorf("empty API-key envelope lost default usage/metadata: %s", body)
				}
			}
			if response["status"] != "failed" || failure["code"] != "gemini_empty_response" || len(output) != 0 ||
				response["id"] == "outer-id" || response["model"] == "outer-model" || bytes.Contains(body, []byte("outer-must-not-leak")) {
				t.Errorf("value=%s chat=%v borrowed outer response fields: %s", value, chat, body)
			}
		}
	}
	for _, wire := range []string{
		`{` + outer + `}`,
		`{` + outer + `,"response":{"candidates":[{"content":{"parts":[{"text":"inner-visible"}]}}]}}`,
		`{"choices":[],"response":{"candidates":[{"content":{"parts":[{"text":"inner-visible"}]}}]}}`,
	} {
		body, err := geminiEnvelopeResponse04384(wire, true)
		want := "outer-must-not-leak"
		if strings.Contains(wire, "inner-visible") {
			want = "inner-visible"
		}
		if err != nil || !bytes.Contains(body, []byte(want)) {
			t.Fatalf("valid absent/object envelope control lost text: %s, %v", body, err)
		}
	}
}

func TestProdex04384GeminiValidNonObjectJSONHasNoResponseFields(t *testing.T) {
	for _, wire := range []string{`null`, `[]`, `[{}]`, `true`, `7`, `"inner"`} {
		for _, chat := range []bool{false, true} {
			body, err := geminiEnvelopeResponse04384(wire, chat)
			if err != nil || !bytes.Contains(body, []byte(`"code":"gemini_empty_response"`)) {
				t.Errorf("valid non-object wire=%s chat=%v body=%s err=%v", wire, chat, body, err)
			}
		}
		state := &geminiStreamState{responseID: "resp-synthetic"}
		var events bytes.Buffer
		if err := state.consume(&events, []byte(wire), nil); err != nil || events.Len() != 0 {
			t.Errorf("valid non-object SSE emitted content or parse failure: %q, %v", events.String(), err)
		}
	}
}

func TestProdex04384GeminiChatFallbackRequiresWholeSerdeDocument(t *testing.T) {
	for _, extra := range []string{`"\ud800"`, `1e309`, `1.7976931348623158e308`, `"` + string([]byte{0xff}) + `"`} {
		wire := `{"choices":[],"unused":` + extra + `}`
		if body, err := geminiEnvelopeResponse04384(wire, true); err == nil {
			t.Errorf("serde-invalid ignored Chat field became an accepted envelope: %s", body)
		}
	}
	for _, extra := range []string{`"\ud83d\ude00"`, `1.79769313486231580e308`} {
		if body, err := geminiEnvelopeResponse04384(`{"choices":[],"unused":`+extra+`}`, true); err != nil || len(body) == 0 {
			t.Errorf("valid Chat fallback control rejected: %s, %v", body, err)
		}
	}
}

func geminiEnvelopeResponse04384(wire string, chat bool) ([]byte, error) {
	upstream := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader(wire))}
	var response *proxymodel.Response
	var err error
	if chat {
		response, err = translateGeminiChatOrNativeResponse043711(upstream)
	} else {
		response, err = translateResponse(upstream, nil)
	}
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	return io.ReadAll(response.Body)
}
