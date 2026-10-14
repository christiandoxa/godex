package gemini

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/sse"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex043711GeminiAPIKeyNativeSSEUsesChatStreamSemantics(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/openai/chat/completions" {
			t.Errorf("Gemini API-key stream request path=%q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: "+`{"candidates":[{"content":{"role":"model","parts":[{"text":"synthetic-"}]}}],"modelVersion":"gemini-3.1-pro-preview"}`+"\n\n")
		_, _ = io.WriteString(w, "data: "+`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1,"totalTokenCount":3}}`+"\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	pool, err := NewRuntimePool(server.URL+"/v1beta", []proxymodel.ProviderCredential{
		{ID: "key-a", Secret: "synthetic-credential"},
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	result, err := pool.Execute(t.Context(), proxymodel.Request{
		Method: http.MethodPost, Path: "/backend-api/prodex/responses",
		Body: []byte(`{"model":"gemini-3.1-pro-preview","input":[{"role":"user","content":"hello"}],"stream":true}`),
	}, proxymodel.Account{ID: "key-a"})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Body.Close()
	body, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatal(err)
	}
	if captured["stream"] != true {
		t.Fatalf("stream flag lost: %+v", captured)
	}
	options, _ := captured["stream_options"].(map[string]any)
	if options["include_usage"] != true {
		t.Fatalf("tagged Gemini SSE upstream must set include_usage: %+v", captured)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %s", result.StatusCode, body)
	}
	decoder := sse.NewDecoder(1 << 20)
	wire := append(decoder.Feed(body), decoder.Finish()...)
	var gotTypes []string
	for index, raw := range wire {
		var event map[string]any
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatalf("invalid stream event %d: %q (%v)", index, raw, err)
		}
		eventType, _ := event["type"].(string)
		gotTypes = append(gotTypes, eventType)
		if event["sequence_number"] != float64(index) {
			t.Fatalf("tagged sequence=%v at index %d: %s", event["sequence_number"], index, raw)
		}
		if index == len(wire)-1 {
			response, _ := event["response"].(map[string]any)
			output, _ := response["output"].([]any)
			if output == nil || len(output) != 0 {
				t.Fatalf("native SSE was falsely surfaced as Chat Completions text: %s", raw)
			}
		}
	}
	want := []string{"response.created", "response.completed"}
	if !reflect.DeepEqual(gotTypes, want) {
		t.Fatalf("tagged Gemini chat-compatible stream event types=%v, want=%v body=%s", gotTypes, want, body)
	}
}
