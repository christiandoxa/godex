package gemini

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiResponsesIgnoreChatMessages(t *testing.T) {
	for _, value := range []string{"null", "false", `{"role":"user","content":"ignored"}`, `[{"role":"system","content":"ignored"}]`, `"ignored"`} {
		for _, input := range []string{`,"input":"same request"`, ""} {
			t.Run(value+input, func(t *testing.T) {
				requests := make(chan string, 2)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
					}
					requests <- string(body)
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"synthetic-ok"}]}}]}`)
				}))
				defer server.Close()
				pool, err := NewRuntimePool(server.URL, []proxymodel.ProviderCredential{{ID: "primary", Secret: "synthetic-key"}}, server.Client())
				if err != nil {
					t.Fatal(err)
				}
				defer pool.Close()
				body := `{"model":"gemini-3.1-pro-preview","messages":` + value + input + `}`
				response, err := pool.Execute(t.Context(), proxymodel.Request{Method: http.MethodPost,
					Path: "/backend-api/godex/responses", Body: []byte(body)}, proxymodel.Account{ID: "primary"})
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				text := "same request"
				if input == "" {
					text = ""
				}
				want := `{"messages":[{"content":"` + text + `","role":"user"}],"model":"gemini-3.1-pro-preview","stream":false}`
				if response.StatusCode != 200 || len(requests) != 1 {
					t.Fatalf("ignored messages status=%d attempts=%d", response.StatusCode, len(requests))
				}
				if got := <-requests; got != want {
					t.Fatalf("ignored messages changed Responses input: got=%s want=%s", got, want)
				}
			})
		}
	}
}
