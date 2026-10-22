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

func TestProdex04384GeminiCompactReplaysCompletedTextStream(t *testing.T) {
	for _, fixture := range []struct {
		name, account                                    string
		incomplete, unknown, providerError, complexDelta bool
	}{
		{name: "completed"}, {name: "incomplete", incomplete: true},
		{name: "unbound", unknown: true}, {name: "different credential", account: "secondary"},
		{name: "provider failure", providerError: true},
		{name: "complex stream keeps existing uncached path", complexDelta: true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			requests := make(chan map[string]any, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request map[string]any
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				requests <- request
				if request["stream"] == true {
					w.Header().Set("Content-Type", "text/event-stream")
					if fixture.complexDelta {
						_, _ = io.WriteString(w, "data: "+`{"choices":[{"delta":{"reasoning_content":"synthetic reasoning"}}]}`+"\n\n")
					}
					_, _ = io.WriteString(w, "data: "+`{"choices":[{"delta":{"content":"cached stream text"}}]}`+"\n\n")
					if fixture.providerError {
						_, _ = io.WriteString(w, "data: "+`{"error":{"message":"synthetic stream failure"}}`+"\n\n")
					}
					if !fixture.incomplete {
						_, _ = io.WriteString(w, "data: [DONE]\n\n")
					}
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"output_text":"synthetic summary"}`)
			}))
			defer server.Close()
			pool, err := NewRuntimePool(server.URL, []proxymodel.ProviderCredential{{ID: "primary", Secret: "synthetic-primary"}, {ID: "secondary", Secret: "synthetic-secondary"}}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			seed, err := pool.Execute(t.Context(), proxymodel.Request{Method: "POST", Path: mountPath + "/responses", Body: []byte(`{"input":"stream request","stream":true}`)}, proxymodel.Account{ID: "primary"})
			if err != nil {
				t.Fatal(err)
			}
			wire, err := io.ReadAll(seed.Body)
			_ = seed.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			decoder := sse.NewDecoder(1 << 20)
			frames := append(decoder.Feed(wire), decoder.Finish()...)
			var publicID string
			for _, frame := range frames {
				var event struct {
					Response struct{ ID string } `json:"response"`
				}
				if json.Unmarshal(frame, &event) == nil && event.Response.ID != "" {
					publicID = event.Response.ID
				}
			}
			if publicID == "" {
				t.Fatal("stream lost its public response ID")
			}
			if fixture.unknown {
				publicID = "resp-unknown"
			}
			body, _ := json.Marshal(map[string]any{"previous_response_id": publicID, "input": []any{}})
			account := fixture.account
			if account == "" {
				account = "primary"
			}
			response, err := pool.Execute(t.Context(), proxymodel.Request{Method: "POST", Path: mountPath + "/responses/compact", Body: body}, proxymodel.Account{ID: account})
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if len(requests) != 2 || response.StatusCode != 200 {
				t.Fatal("Compact changed attempt or response status")
			}
			<-requests
			actual := <-requests
			want := []any{map[string]any{"role": "system", "content": compactInstructions}}
			if !fixture.incomplete && !fixture.unknown && !fixture.providerError && !fixture.complexDelta && fixture.account == "" {
				want = append(want, map[string]any{"role": "user", "content": "stream request"}, map[string]any{"role": "assistant", "content": "cached stream text"})
			}
			want = append(want, map[string]any{"role": "user", "content": compactInstructions})
			if !reflect.DeepEqual(actual["messages"], want) {
				t.Fatalf("stream replay=%#v, want %#v", actual["messages"], want)
			}
		})
	}
}
