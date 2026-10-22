package gemini

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiChatFunctionDeclarationsPreserveCompletePipeline(t *testing.T) {
	const base = `{"function":{"name":"synthetic_lookup","parameters":{"type":"object"}},"type":"function"}`
	const generic = `{"function":{"name":"synthetic_lookup","parameters":{"additionalProperties":true,"type":"object"}},"type":"function"}`
	const specific = `{"function":{"name":"synthetic_lookup","parameters":{"properties":{},"required":[],"type":"object"}},"type":"function"}`
	other := strings.Replace(base, "synthetic_lookup", "other_lookup", 1)
	withDefault := func(value string) string {
		return strings.Replace(base, `"type":"object"`, `"default":`+value+`,"type":"object"`, 1)
	}
	var many []string
	for index := range 129 {
		many = append(many, strings.Replace(base, "synthetic_lookup", fmt.Sprintf("synthetic_%03d", index), 1))
	}
	for _, fixture := range []struct {
		name, input, wire string
		reject            bool
	}{
		{name: "empty", input: "[]"},
		{name: "flat", input: `[{"type":"function","name":"synthetic_lookup","parameters":{"type":"object"}}]`, wire: "[" + base + "]"},
		{name: "nested-control", input: "[" + base + "]", wire: "[" + base + "]"},
		{name: "description-empty", input: "[" + strings.Replace(base, `"name":`, `"description":"","name":`, 1) + "]", wire: "[" + base + "]"},
		{name: "description-blank", input: "[" + strings.Replace(base, `"name":`, `"description":" \u2003","name":`, 1) + "]", wire: "[" + base + "]"},
		{name: "description-null", input: "[" + strings.Replace(base, `"name":`, `"description":null,"name":`, 1) + "]", reject: true},
		{name: "extra-function", input: "[" + strings.Replace(base, `"name":`, `"extra":1.0,"name":`, 1) + "]", wire: "[" + base + "]"},
		{name: "strict-false", input: "[" + strings.Replace(base, `"name":`, `"strict":false,"name":`, 1) + "]", wire: "[" + strings.Replace(base, `"parameters":{"type":"object"}`, `"parameters":{"type":"object"},"strict":false`, 1) + "]"},
		{name: "strict-true", input: "[" + strings.Replace(base, `"name":`, `"strict":true,"name":`, 1) + "]", reject: true},
		{name: "strict-string", input: "[" + strings.Replace(base, `"name":`, `"strict":"true","name":`, 1) + "]", reject: true},
		{name: "name-invalid", input: "[" + strings.Replace(base, "synthetic_lookup", "invalid.name", 1) + "]", reject: true},
		{name: "name-over", input: "[" + strings.Replace(base, "synthetic_lookup", strings.Repeat("n", 129), 1) + "]", reject: true},
		{name: "identical", input: "[" + base + "," + base + "]", wire: "[" + base + "]"},
		{name: "equivalent-numbers", input: "[" + withDefault("1000.0") + "," + withDefault("1e3") + "]", wire: "[" + withDefault("1000.0") + "]"},
		{name: "floating-zero", input: "[" + withDefault("-0.0") + "," + withDefault("0.0") + "]", wire: "[" + withDefault("-0.0") + "]"},
		{name: "quoted-zero", input: "[" + withDefault(`"-0.0"`) + "," + withDefault(`"0.0"`) + "]", reject: true},
		{name: "integer-zero", input: "[" + withDefault("0") + "," + withDefault("0.0") + "]", reject: true},
		{name: "conflict", input: "[" + base + "," + withDefault("1") + "]", reject: true},
		{name: "generic-first", input: "[" + generic + "," + specific + "]", wire: "[" + specific + "]"},
		{name: "generic-last", input: "[" + specific + "," + generic + "]", wire: "[" + specific + "]"},
		{name: "generic-stable-order", input: "[" + generic + "," + other + "," + specific + "]", wire: "[" + other + "," + specific + "]"},
		{name: "generic-null-presence", input: "[" + generic + "," + strings.Replace(generic, `"type":"object"`, `"properties":null,"type":"object"`, 1) + "]", wire: "[" + strings.Replace(generic, `"type":"object"`, `"properties":null,"type":"object"`, 1) + "]"},
		{name: "count-limit", input: "[" + strings.Join(many[:128], ",") + "]", wire: "[" + strings.Join(many[:128], ",") + "]"},
		{name: "count-over", input: "[" + strings.Join(many, ",") + "]", reject: true},
		{name: "count-after-dedup", input: "[" + strings.TrimSuffix(strings.Repeat(base+",", 129), ",") + "]", wire: "[" + base + "]"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			requests := make(chan string, 8)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				requests <- string(body)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"synthetic-ok"}]},"finishReason":"STOP"}]}`)
			}))
			defer server.Close()
			pool, err := NewRuntimePool(server.URL, []proxymodel.ProviderCredential{{ID: "primary", Secret: "synthetic-key"}}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			body := `{"model":"gemini-3.1-pro-preview","input":"same request","tools":` + fixture.input + `}`
			response, err := pool.Execute(t.Context(), proxymodel.Request{Method: "POST", Path: "/backend-api/godex/responses", Body: []byte(body)}, proxymodel.Account{ID: "primary"})
			if fixture.reject {
				var failure *proxymodel.Error
				if response != nil || !errors.As(err, &failure) || failure.StatusCode != 502 || len(requests) != 0 {
					t.Fatal("invalid declarations lost terminal no-dispatch failure")
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
			wire := `{"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stream":false`
			if fixture.wire != "" {
				wire += `,"tools":` + fixture.wire
			}
			if response.StatusCode != 200 || len(requests) != 1 || <-requests != wire+"}" || !strings.Contains(string(payload), "synthetic-ok") {
				t.Fatal("declarations lost exact wire, stable survivors, attempt count or output")
			}
		})
	}
}
