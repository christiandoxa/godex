package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

const historyInvalid04380 = `{"error":{"type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `."}}`
const historyRecovered04380 = `{"object":"response","id":"resp-history-recovered"}`

type historyCase04380 struct {
	name, input string
	replay      bool
}

var historyCases04380 = []historyCase04380{
	{"compaction", `[{"type":"compaction"},{"role":"user"}]`, true},
	{"escaped-compaction", `[{"type":"compact\u0069on"},{"role":"user"}]`, true},
	{"padded-compaction", `[{"type":" compaction "},{"role":"user"}]`, false},
	{"compaction-trigger", `[{"type":"compaction_trigger"},{"role":"user"}]`, false},
	{"completed-turn", `[{"role":"user"},{"role":"assistant"},{"role":"user"}]`, true},
	{"padded-user", `[{"role":" user "},{"role":"assistant"},{"role":"user"}]`, false},
	{"padded-assistant", `[{"role":"user"},{"role":"assistant\t"},{"role":"user"}]`, false},
	{"completed-call", `[{"role":"user"},{"type":"function_call"},{"role":"user"}]`, true},
	{"padded-call", `[{"role":"user"},{"type":"function_call "},{"role":"user"}]`, false},
}

type historyAttempt04380 struct {
	owner string
	body  map[string]any
}

type historyObservation04380 struct {
	seedOwner string
	seeds     int
	attempts  []historyAttempt04380
	response  exchange
}

func TestProdex04380ExecutableHistoryReplayBoundary(t *testing.T) {
	prodex, godex := os.Getenv("GODEX_TEST_HISTORY_PRODEX_BIN"), os.Getenv("GODEX_TEST_HISTORY_GODEX_BIN")
	if prodex == "" && godex == "" {
		t.Skip("set explicit pinned history oracle executable and source paths")
	}
	if err := verifyProdex(prodex, os.Getenv("GODEX_TEST_HISTORY_PRODEX_SOURCE")); err != nil {
		t.Fatal(err)
	}
	if err := verifyWebSocketCandidate(godex, os.Getenv("GODEX_TEST_HISTORY_GODEX_SOURCE"), os.Getenv("GODEX_TEST_HISTORY_GODEX_COMMIT")); err != nil {
		t.Fatal(err)
	}
	// Prodex's private capability location has a bounded path length.
	root, err := os.MkdirTemp("", "gdx-hist-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	for _, spec := range []product{{name: "prodex", binary: prodex}, {name: "godex", binary: godex}} {
		t.Run(spec.name, func(t *testing.T) { runHistoryBoundary04380(t, root, spec) })
	}
}

func runHistoryBoundary04380(t *testing.T, root string, p product) {
	t.Helper()
	if err := prepareProduct(root, &p); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	observations := make(map[string]*historyObservation04380)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/wham/usage") {
			writeJSON(w, 200, `{"plan_type":"plus","rate_limit":{"primary_window":{"used_percent":10},"secondary_window":{"used_percent":10}}}`)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		var value map[string]any
		owner := r.Header.Get("ChatGPT-Account-Id")
		token := map[string]string{"account-a": "token-a", "account-b": "token-b"}[owner]
		if err != nil || json.Unmarshal(body, &value) != nil || r.Method != http.MethodPost ||
			!strings.HasSuffix(r.URL.Path, "/responses") || token == "" ||
			r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "invalid synthetic request", 400)
			return
		}
		name, _ := value["fixture"].(string)
		mu.Lock()
		defer mu.Unlock()
		observation := observations[name]
		if observation == nil {
			observation = &historyObservation04380{}
			observations[name] = observation
		}
		if value["mode"] == "history-seed" {
			observation.seeds++
			observation.seedOwner = owner
			writeJSON(w, 200, fmt.Sprintf(`{"object":"response","id":%q}`, "resp-history-"+name))
			return
		}
		observation.attempts = append(observation.attempts, historyAttempt04380{owner, value})
		if _, previous := value["previous_response_id"]; previous {
			writeJSON(w, 400, historyInvalid04380)
			return
		}
		writeJSON(w, 200, historyRecovered04380)
	}))
	defer upstream.Close()
	endpoint, command, err := startGateway(p, upstream.URL+"/backend-api")
	if err != nil {
		t.Fatal(err)
	}
	defer stopGateway(command)
	client := &http.Client{Timeout: 8 * time.Second}
	for _, fixture := range historyCases04380 {
		t.Run(fixture.name, func(t *testing.T) {
			seed := fmt.Sprintf(`{"mode":"history-seed","fixture":%q,"session_id":%q,"input":"seed history owner"}`, fixture.name, "session-"+fixture.name)
			seedBody := fmt.Sprintf(`{"object":"response","id":%q}`, "resp-history-"+fixture.name)
			if response, err := post(client, endpoint+"/responses", seed); err != nil || response.status != 200 || response.body != seedBody {
				t.Fatalf("seed response: status=%d err=%v", response.status, err)
			}
			original := fmt.Sprintf(`{"mode":"history-recover","fixture":%q,"model":"gpt-5.6","session_id":%q,"previous_response_id":%q,"context_window_id":"window-before","client_metadata":{"context_window_id":"window-before"},"input":%s}`, fixture.name, "session-"+fixture.name, "resp-history-"+fixture.name, fixture.input)
			response, err := post(client, endpoint+"/responses", original)
			if err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			current := observations[fixture.name]
			if current == nil {
				mu.Unlock()
				t.Fatal("upstream did not observe a seeded history request")
			}
			observation := *current
			observation.attempts = append([]historyAttempt04380(nil), observation.attempts...)
			mu.Unlock()
			observation.response = response
			var request map[string]any
			if err := json.Unmarshal([]byte(original), &request); err != nil {
				t.Fatal(err)
			}
			if err := validateHistoryObservation04380(observation, request, fixture.replay); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func validateHistoryObservation04380(observed historyObservation04380, original map[string]any, replay bool) error {
	wantAttempts, wantStatus, wantBody := 1, 400, historyInvalid04380
	if replay {
		wantAttempts, wantStatus, wantBody = 2, 200, historyRecovered04380
	}
	if observed.seeds != 1 || observed.seedOwner == "" || len(observed.attempts) != wantAttempts ||
		observed.response.status != wantStatus || observed.response.body != wantBody {
		return fmt.Errorf("history result: seeds=%d attempts=%d status=%d; want seeds=1 attempts=%d status=%d and exact response", observed.seeds, len(observed.attempts), observed.response.status, wantAttempts, wantStatus)
	}
	for index, attempt := range observed.attempts {
		expected := make(map[string]any, len(original))
		for key, value := range original {
			expected[key] = value
		}
		if index > 0 {
			delete(expected, "previous_response_id")
		}
		if attempt.owner != observed.seedOwner || !reflect.DeepEqual(attempt.body, expected) {
			return fmt.Errorf("history attempt %d changed owner or request fields", index+1)
		}
	}
	return nil
}

func TestProdex04380HistoryOracleRejectsMutations(t *testing.T) {
	original := map[string]any{"previous_response_id": "resp-original", "input": "keep"}
	valid := func() historyObservation04380 {
		return historyObservation04380{seedOwner: "owner", seeds: 1,
			attempts: []historyAttempt04380{{owner: "owner", body: original}},
			response: exchange{status: 400, body: historyInvalid04380}}
	}
	if err := validateHistoryObservation04380(valid(), original, false); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*historyObservation04380){
		"missing seed":  func(o *historyObservation04380) { o.seeds = 0 },
		"replay":        func(o *historyObservation04380) { o.attempts = append(o.attempts, o.attempts[0]) },
		"foreign owner": func(o *historyObservation04380) { o.attempts[0].owner = "foreign" },
		"changed body": func(o *historyObservation04380) {
			o.attempts[0].body = map[string]any{"previous_response_id": "wrong", "input": "keep"}
		},
		"false success": func(o *historyObservation04380) { o.response.status = 200 },
		"changed error": func(o *historyObservation04380) { o.response.body = "changed" },
	} {
		t.Run(name, func(t *testing.T) {
			observed := valid()
			mutate(&observed)
			if err := validateHistoryObservation04380(observed, original, false); err == nil {
				t.Fatal("mutated history observation passed")
			}
		})
	}
}

func TestProdex04380HistoryOracleRequiresUnchangedReplayContext(t *testing.T) {
	original := map[string]any{"previous_response_id": "resp-original", "input": "keep", "context_window_id": "window-original"}
	valid := func() historyObservation04380 {
		return historyObservation04380{seedOwner: "owner", seeds: 1,
			attempts: []historyAttempt04380{
				{owner: "owner", body: original},
				{owner: "owner", body: map[string]any{"input": "keep", "context_window_id": "window-original"}},
			}, response: exchange{status: 200, body: historyRecovered04380}}
	}
	if err := validateHistoryObservation04380(valid(), original, true); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*historyObservation04380){
		"missing recovery": func(o *historyObservation04380) { o.attempts = o.attempts[:1] },
		"retained ID":      func(o *historyObservation04380) { o.attempts[1].body["previous_response_id"] = "resp-original" },
		"changed context":  func(o *historyObservation04380) { o.attempts[1].body["context_window_id"] = "wrong" },
		"changed owner":    func(o *historyObservation04380) { o.attempts[1].owner = "foreign" },
	} {
		t.Run(name, func(t *testing.T) {
			observation := valid()
			mutate(&observation)
			if err := validateHistoryObservation04380(observation, original, true); err == nil {
				t.Fatal("mutated history replay passed")
			}
		})
	}
}
