package copilot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestProdex04380CopilotCandidateCountRejectsExcessiveLoggedUsersBeforeAuth(t *testing.T) {
	for _, tc := range []struct {
		name     string
		hasLast  bool
		logged   int
		rejected bool
	}{
		{name: "256 logged without last is valid", logged: 256},
		{name: "257 logged without last is invalid", logged: 257, rejected: true},
		{name: "255 logged with last is valid", logged: 255, hasLast: true},
		{name: "256 logged with last is invalid", logged: 256, hasLast: true, rejected: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var networkCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				networkCalls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"login":"synthetic"}`))
			}))
			defer server.Close()
			home := t.TempDir()
			users := make([]configUser, tc.logged)
			for index := range users {
				users[index] = configUser{Host: server.URL, Login: "synthetic"}
			}
			config := configFile{LoggedInUsers: users, CopilotTokens: map[string]string{
				server.URL + ":synthetic": "synthetic-token-not-real",
			}}
			if tc.hasLast {
				config.LastLoggedInUser = &configUser{Host: server.URL, Login: "synthetic"}
			}
			encoded, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(home, "config.json"), encoded, 0600); err != nil {
				t.Fatal(err)
			}
			source := NewSource(server.Client())
			source.getenv = func(key string) string {
				if key == "COPILOT_HOME" {
					return home
				}
				return ""
			}
			result, err := source.Load(context.Background())
			if tc.rejected {
				if err == nil || !strings.Contains(err.Error(), "candidate") || networkCalls.Load() != 0 {
					t.Fatalf("oversized Copilot candidate list was not rejected before auth: err=%v result=%+v calls=%d", err, result, networkCalls.Load())
				}
				return
			}
			if err != nil || networkCalls.Load() != 1 || result.Provider.Kind != "copilot" {
				t.Fatalf("valid Copilot candidate list rejected: err=%v calls=%d", err, networkCalls.Load())
			}
		})
	}
}

func TestProdex04380CopilotCandidateOrderUsesExactHostAndLogin(t *testing.T) {
	config := configFile{
		LastLoggedInUser: &configUser{Host: "https://github.example", Login: "Alice"},
		LoggedInUsers: []configUser{
			{Host: "https://github.example", Login: "Alice"},
			{Host: "https://github.example", Login: "alice"},
			{Host: "https://github.example/", Login: "Alice"},
			{Host: "https://github.example", Login: " Alice"},
			{Host: "https://github.example", Login: "Alice"},
		},
	}
	candidates := candidateUsers(config)
	if len(candidates) != 4 || candidates[0] != *config.LastLoggedInUser ||
		candidates[1].Login != "alice" || candidates[2].Host != "https://github.example/" ||
		candidates[3].Login != " Alice" {
		t.Fatalf("tagged exact (host,login) ordering/dedup changed: %+v", candidates)
	}
}
