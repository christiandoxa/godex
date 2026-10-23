package codex

import (
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"
)

func TestCodexAuthOriginatorValidatesFieldValue(t *testing.T) {
	for _, fixture := range []struct{ name, value, want string }{
		{"custom", "codex-tui", "codex-tui"},
		{"empty", "", ""},
		{"tab", "codex\ttui", "codex\ttui"},
		{"unicode", "codex-β", "codex-β"},
		{"newline", "invalid\nvalue", "codex_cli_rs"},
		{"carriage return", "invalid\rvalue", "codex_cli_rs"},
		{"control", "invalid\x01value", "codex_cli_rs"},
		{"delete", "invalid\x7fvalue", "codex_cli_rs"},
		{"invalid UTF-8", "invalid\xffvalue", "codex_cli_rs"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			if fixture.name == "invalid UTF-8" && runtime.GOOS == "windows" {
				t.Skip("Windows environment uses UTF-16")
			}
			t.Setenv("CODEX_INTERNAL_ORIGINATOR_OVERRIDE", fixture.value)
			if got := codexAuthOriginator(); got != fixture.want {
				t.Fatalf("originator=%q want=%q", got, fixture.want)
			}
		})
	}
	t.Run("unset", func(t *testing.T) {
		t.Setenv("CODEX_INTERNAL_ORIGINATOR_OVERRIDE", "temporary")
		if err := os.Unsetenv("CODEX_INTERNAL_ORIGINATOR_OVERRIDE"); err != nil {
			t.Fatal(err)
		}
		if got := codexAuthOriginator(); got != "codex_cli_rs" {
			t.Fatalf("unset originator=%q", got)
		}
	})
}

func TestChatGPTRefreshHonorsCodexOriginator(t *testing.T) {
	t.Setenv("CODEX_INTERNAL_ORIGINATOR_OVERRIDE", "codex-tui")
	observed := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed <- r.Header.Get("originator")
		_, _ = w.Write([]byte(`{"access_token":"synthetic-refreshed"}`))
	}))
	defer server.Close()
	t.Setenv(refreshTokenOverrideEnv, server.URL)
	if _, err := requestChatGPTAuthRefresh(t.Context(), "synthetic-refresh"); err != nil {
		t.Fatal(err)
	}
	if len(observed) != 1 || <-observed != "codex-tui" {
		t.Fatal("auth refresh lost originator override")
	}
}
