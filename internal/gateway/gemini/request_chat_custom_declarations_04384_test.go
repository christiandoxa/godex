package gemini

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiChatCustomDeclarationsPreserveCompletePipeline(t *testing.T) {
	const freeform = "Call this custom/freeform tool with the exact raw tool input in the `input` string field."
	const patch = "Call this custom/freeform tool with the exact Codex apply_patch grammar in the `input` string field. The first line must be `*** Begin Patch`, the last line must be `*** End Patch`, and hunks must use `*** Add File:`, `*** Delete File:`, or `*** Update File:`. For `*** Add File: path`, every new file content line must start with `+`; for example `+hello`, and a blank content line is `+`. Do not pass unified diff headers such as `--- a/...` or `+++ b/...` as the top-level input."
	for _, fixture := range []struct {
		name, fields, description, format string
		status                            int
	}{
		{name: "synthetic_custom"},
		{name: "apply_patch"},
		{name: "Apply_Patch"},
		{name: "description", fields: `,"description":" synthetic "`, description: " synthetic "},
		{name: "blank-description", fields: `,"description":" \t\u2003"`},
		{name: "strict-disabled", fields: `,"strict":false`},
		{name: "empty-format", fields: `,"format":{}`, format: "{}"},
		{name: "canonical-format", fields: `,"format":{"type":"grammar","number":1e3,"decimal":1.0,"text":"<&/🌍\u2028\u2029","u64":18446744073709551615}`, format: `{"decimal":1.0,"number":1000.0,"text":"<&/🌍  ","type":"grammar","u64":18446744073709551615}`},
		{name: "description-null", fields: `,"description":null`, status: 502},
		{name: "strict-enabled", fields: `,"strict":true`, status: 502},
		{name: "format-null", fields: `,"format":null`, status: 502},
		{name: "format-type-number", fields: `,"format":{"type":7}`, status: 502},
		{name: "", status: 400},
		{name: "invalid.name", status: 502},
	} {
		for _, stream := range []bool{false, true} {
			mode, streamValue := "native", "false"
			if stream {
				mode, streamValue = "stream", "true"
			}
			t.Run(fixture.name+"/"+mode, func(t *testing.T) {
				requests := make(chan string, 8)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
					}
					requests <- string(body)
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: "+`{"choices":[{"delta":{"content":"synthetic-ok"},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"synthetic-ok"}]},"finishReason":"STOP"}]}`)
				}))
				defer server.Close()
				pool, err := NewRuntimePool(server.URL, []proxymodel.ProviderCredential{{ID: "primary", Secret: "synthetic-key"}}, server.Client())
				if err != nil {
					t.Fatal(err)
				}
				defer pool.Close()
				body := `{"model":"gemini-3.1-pro-preview","input":"same request","stream":` + streamValue + `,"tools":[{"type":"custom","name":"` + fixture.name + `"` + fixture.fields + `}]}`
				response, err := pool.Execute(t.Context(), proxymodel.Request{Method: "POST", Path: "/backend-api/godex/responses", Body: []byte(body)}, proxymodel.Account{ID: "primary"})
				if fixture.status != 0 {
					var failure *proxymodel.Error
					if response != nil || !errors.As(err, &failure) || failure.StatusCode != fixture.status || len(requests) != 0 {
						t.Fatalf("custom declaration lost validation/no-dispatch: error=%v attempts=%d", err, len(requests))
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				payload, err := io.ReadAll(response.Body)
				if err != nil {
					t.Fatal(err)
				}
				if response.StatusCode != 200 || len(requests) != 1 || !strings.Contains(string(payload), "synthetic-ok") {
					t.Fatal("custom declaration lost attempt count, status or response")
				}
				var request struct {
					Tools []struct {
						Type     string
						Function struct {
							Name, Description string
							Parameters        json.RawMessage
						}
					}
				}
				if err := json.Unmarshal([]byte(<-requests), &request); err != nil || len(request.Tools) != 1 {
					t.Fatal("custom declaration did not become one function tool")
				}
				description := fixture.description
				if description == "" {
					description = "Freeform custom tool input."
				}
				guidance, parameterDescription := freeform, "Exact raw input for the custom/freeform tool."
				if fixture.name == "apply_patch" {
					guidance, parameterDescription = patch, "Exact raw apply_patch input. For Add File, prefix every new file content line with '+'."
				}
				description += "\n\n" + guidance
				if fixture.format != "" {
					description += "\n\nOriginal custom tool format JSON: " + fixture.format
				}
				parameters := `{"additionalProperties":false,"properties":{"input":{"description":"` + parameterDescription + `","type":"string"}},"required":["input"],"type":"object"}`
				got := request.Tools[0]
				if got.Type != "function" || got.Function.Name != fixture.name || got.Function.Description != description || string(got.Function.Parameters) != parameters {
					t.Fatal("custom declaration lost exact guidance, embedded format or input schema")
				}
			})
		}
	}
}
