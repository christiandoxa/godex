package gemini

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiChatUserIDUsesTaggedAliasesAndValidation(t *testing.T) {
	quote := func(value string) string { encoded, _ := json.Marshal(value); return string(encoded) }
	const invalidType = "Gemini OpenAI-compatible user_id must be a string"
	const invalidValue = "Gemini OpenAI-compatible user_id must use only letters, numbers, underscores, or dashes and be at most 512 bytes"
	for _, fixture := range []struct{ name, fields, want, message string }{
		{"absent", "", "", ""},
		{"canonical", `"user_id":" user_04384 ","user":"ignored_user","safety_identifier":17`, "user_04384", ""},
		{"user_alias", `"user":" user_04384 "`, "user_04384", ""},
		{"safety_alias", `"safety_identifier":" user_04384 "`, "user_04384", ""},
		{"blank_blocks_alias", `"user_id":" \t ","user":"ignored_user"`, "", ""},
		{"unicode_trim_limit", `"user_id":` + quote(" \u2003"+strings.Repeat("u", 512)+"\u00a0 "), strings.Repeat("u", 512), ""},
		{"null_blocks_alias", `"user_id":null,"user":"ignored_user"`, "", invalidType},
		{"number", `"user_id":7`, "", invalidType},
		{"over_limit", `"user_id":` + quote(strings.Repeat("u", 513)), "", invalidValue},
		{"invalid_character", `"user_id":"user.has.dot"`, "", invalidValue},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			var mu sync.Mutex
			var captured map[string]any
			var attempts int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request map[string]any
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				mu.Lock()
				captured, attempts = request, attempts+1
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"synthetic-ok"}]}}]}`)
			}))
			defer server.Close()
			pool, err := NewRuntimePool(server.URL+"/v1beta", []proxymodel.ProviderCredential{{ID: "synthetic", Secret: "synthetic-only"}}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			body := `{"input":"same request","model":"gemini-3.1-pro-preview"`
			if fixture.fields != "" {
				body += "," + fixture.fields
			}
			reply, err := pool.Execute(t.Context(), proxymodel.Request{Method: http.MethodPost,
				Path: "/backend-api/godex/responses", Body: []byte(body + "}")}, proxymodel.Account{ID: "synthetic"})
			if reply != nil {
				defer reply.Body.Close()
			}
			mu.Lock()
			defer mu.Unlock()
			if fixture.message != "" {
				var failure *proxymodel.Error
				if !errors.As(err, &failure) || failure.StatusCode != 502 || failure.Message != "upstream request failed" || attempts != 0 {
					t.Fatalf("invalid user ID was not rejected before upstream: attempts=%d error=%v", attempts, err)
				}
				return
			}
			if err != nil || reply == nil || reply.StatusCode != 200 || attempts != 1 {
				t.Fatalf("valid user ID did not produce one successful request: attempts=%d error=%v", attempts, err)
			}
			value, present := captured["user_id"]
			if present != (fixture.want != "") || present && value != fixture.want {
				t.Fatal("canonical user ID was lost, changed or replaced by a lower-priority alias")
			}
			for _, alias := range []string{"user", "safety_identifier"} {
				if _, present := captured[alias]; present {
					t.Fatalf("non-canonical user ID alias %s reached upstream", alias)
				}
			}
		})
	}
}
