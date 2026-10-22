package deepseek

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384DeepSeekJSONAdmissionReturnsTerminalOutcome(t *testing.T) {
	const request = `{"model":"deepseek-v4-pro","input":"same request"`
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"chatcmpl-json-fixture","choices":[{"message":{"role":"assistant","content":"accepted"}}]}`)
	}))
	defer server.Close()
	for _, fixture := range []struct {
		name, body string
		status     int
	}{
		{"unicode_control", request + `,"unused":"\ud83d\ude00"}`, 200},
		{"null", "null", 500},
		{"array", "[]", 500},
		{"number", "7", 500},
		{"boolean", "true", 500},
		{"string", `"opaque"`, 500},
		{"syntax", `{"input":`, 502},
		{"trailing", request + `} {}`, 502},
		{"surrogate", request + `,"unused":"\ud800"}`, 502},
		{"utf8", request + ",\"unused\":\"\xff\"}", 502},
		{"overflow", request + `,"unused":1e309}`, 502},
		{"root_overflow", "1e309", 502},
		{"depth", request + `,"unused":` + strings.Repeat("[", 140) + "0" + strings.Repeat("]", 140) + "}", 502},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			calls.Store(0)
			transport, err := NewRuntimeTransport(server.URL, "synthetic-json-key", server.Client())
			if err != nil {
				t.Fatal(err)
			}
			defer transport.Close()
			response, err := transport.Execute(t.Context(), proxymodel.Request{Method: http.MethodPost,
				Path: mountPath + "/responses", Body: []byte(fixture.body)}, proxymodel.Account{})
			if response != nil {
				defer response.Body.Close()
			}
			wantCalls := int32(0)
			if fixture.status == 200 {
				wantCalls = 1
			}
			if calls.Load() != wantCalls {
				t.Fatalf("JSON admission attempted upstream %d times, want %d", calls.Load(), wantCalls)
			}
			if fixture.status == 502 {
				var failure *proxymodel.Error
				if response != nil || !errors.As(err, &failure) || failure.StatusCode != 502 || failure.Message != "upstream request failed" {
					t.Fatalf("malformed body lost terminal preparation error: %v", err)
				}
				return
			}
			if err != nil || response == nil || response.StatusCode != fixture.status {
				t.Fatalf("JSON admission outcome differs: error=%v response=%v", err, response)
			}
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if fixture.status == 500 {
				if len(body) != 0 || len(response.Header["Content-Type"]) != 0 || response.PrecommitFailure == nil || response.PrecommitFailure.Code != "invalid_request_error" {
					t.Fatal("nonobject root lost exact empty terminal 500")
				}
			} else if !strings.Contains(string(body), "accepted") {
				t.Fatal("valid Unicode control lost its response")
			}
		})
	}
}
