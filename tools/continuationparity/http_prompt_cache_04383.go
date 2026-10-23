package main

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

var httpPromptCacheShapes04383 = []string{
	"direct", "nested", "zero", "legacy", "overflow-fallback", "max-u64",
	"nonfinite-unrelated", "finite-unrelated", "underflow-unrelated",
	"overdeep-unrelated", "max-valid-depth",
}

func httpPromptCacheRequest04383(stage string) string {
	switch stage {
	case "seed-main":
		return `{"model":"gpt-6","previous_response_id":"resp-main","prompt_cache_key":"cache-affinity-synthetic","input":[{"role":"user","content":"cache main"}]}`
	case "seed-second":
		return `{"model":"gpt-6","previous_response_id":"resp-ready-b","prompt_cache_key":"cache-affinity-synthetic","input":[{"role":"user","content":"cache second"}]}`
	case "restore-main":
		return `{"model":"gpt-6","previous_response_id":"resp-main","input":[{"role":"user","content":"restore main"}]}`
	case "fresh":
		return `{"model":"gpt-6","prompt_cache_key":"cache-affinity-synthetic","input":[{"role":"user","content":"select cache owner"}]}`
	default:
		return ""
	}
}

func httpPromptCacheReply04383(shape, stage string) string {
	id := "resp-cache-" + stage
	if stage == "seed-main" {
		return fmt.Sprintf(`{"id":%q,"object":"response","usage":{"cached_input_tokens":10},"output":[]}`, id)
	}
	if stage == "seed-second" {
		switch shape {
		case "direct":
			return fmt.Sprintf(`{"id":%q,"object":"response","usage":{"cached_input_tokens":50},"output":[]}`, id)
		case "nested":
			return fmt.Sprintf(`{"id":%q,"object":"response","metrics":{"records":[{"usage":{"cached_input_tokens":50}}]},"output":[]}`, id)
		case "zero":
			return fmt.Sprintf(`{"id":%q,"object":"response","usage":{"cached_input_tokens":0,"input_tokens_details":{"cached_tokens":99}},"output":[]}`, id)
		case "legacy":
			return fmt.Sprintf(`{"id":%q,"object":"response","usage":{"prompt_cache_hit_tokens":99},"output":[]}`, id)
		case "overflow-fallback":
			return fmt.Sprintf(`{"id":%q,"object":"response","usage":{"cached_input_tokens":18446744073709551616,"input_tokens_details":{"cached_tokens":60}},"output":[]}`, id)
		case "max-u64":
			return fmt.Sprintf(`{"id":%q,"object":"response","usage":{"cached_input_tokens":18446744073709551615},"output":[]}`, id)
		case "nonfinite-unrelated":
			return fmt.Sprintf(`{"id":%q,"object":"response","usage":{"cached_input_tokens":50},"ignored":1e309,"output":[]}`, id)
		case "finite-unrelated":
			return fmt.Sprintf(`{"id":%q,"object":"response","usage":{"cached_input_tokens":50},"ignored":1e308,"output":[]}`, id)
		case "underflow-unrelated":
			return fmt.Sprintf(`{"id":%q,"object":"response","usage":{"cached_input_tokens":50},"ignored":1e-99999,"output":[]}`, id)
		case "overdeep-unrelated":
			return httpPromptCacheDepthReply04383(id, 127)
		case "max-valid-depth":
			return httpPromptCacheDepthReply04383(id, 126)
		}
	}
	return fmt.Sprintf(`{"id":%q,"object":"response","output":[]}`, id)
}

// The tagged serde_json 1.0.151 wrapper permits 127 nested containers
// total; the outer response object counts as one.
func httpPromptCacheDepthReply04383(id string, nested int) string {
	deep := strings.Repeat(`{"a":`, nested) + `0` + strings.Repeat(`}`, nested)
	return fmt.Sprintf(`{"id":%q,"object":"response","usage":{"cached_input_tokens":50},"ignored":%s,"output":[]}`, id, deep)
}

func httpPromptCacheWantOwner04383(shape, stage string) string {
	switch stage {
	case "seed-second":
		return "account-b"
	case "fresh":
		if shape != "zero" && shape != "legacy" && shape != "nonfinite-unrelated" && shape != "overdeep-unrelated" {
			return "account-b"
		}
	}
	return "account-a"
}

func runHTTPPromptCacheProduct04383(root string, p product) error {
	for _, shape := range httpPromptCacheShapes04383 {
		if err := runHTTPPromptCacheCase04383(filepath.Join(root, shape), p, shape); err != nil {
			return fmt.Errorf("prompt cache %s: %w", shape, err)
		}
	}
	return runHTTPNativeMetadataProduct04390(filepath.Join(root, "native-metadata"), p)
}

type httpPromptCacheResult04383 struct {
	attempts []transportAttempt04383
	replies  []exchange
	invalid  bool
}

func runHTTPPromptCacheCase04383(root string, p product, shape string) error {
	if err := prepareProduct(root, &p); err != nil {
		return err
	}
	if err := seedTransportAlternate04383(p, "http-prompt-cache-"+shape); err != nil {
		return err
	}
	p.env = append(p.env, "PRODEX_RUNTIME_LOG_RECORD=true")
	backend := newTransportBackend04383("http-prompt-cache-" + shape)
	defer backend.server.Close()
	endpoint, stop, err := startTransportLaunch04383(p, backend.server.URL+"/backend-api")
	if err != nil {
		return err
	}
	defer stop()
	client := &http.Client{Timeout: 8 * time.Second}
	for _, previous := range []string{"resp-ready-b", "resp-main"} {
		warmup, err := post(client, endpoint+"/responses", fmt.Sprintf(`{"previous_response_id":%q,"input":[]}`, previous))
		if err != nil || warmup.status != http.StatusOK {
			return fmt.Errorf("prompt cache warmup %s status=%d err=%v", previous, warmup.status, err)
		}
	}
	until := time.Now().Add(5 * time.Second)
	for !transportProfilesReady04383(p, backend) {
		if time.Now().After(until) {
			return errors.New("prompt cache fixture missing two authenticated Ready profiles")
		}
		time.Sleep(10 * time.Millisecond)
	}
	stages := []string{"seed-main", "seed-second", "restore-main", "fresh"}
	var replies []exchange
	for _, stage := range stages {
		reply, err := post(client, endpoint+"/responses", httpPromptCacheRequest04383(stage))
		if err != nil {
			return fmt.Errorf("prompt cache %s response: %w", stage, err)
		}
		replies = append(replies, reply)
	}
	backend.mu.Lock()
	result := httpPromptCacheResult04383{invalid: backend.invalid,
		attempts: append([]transportAttempt04383(nil), backend.attempts...), replies: replies}
	backend.mu.Unlock()
	if err := verifyHTTPPromptCache04383(shape, result); err != nil {
		return err
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		ready := true
		for _, stage := range stages {
			owner, err := readHTTPResponseOwner04383(p, "resp-cache-"+stage)
			want := "main"
			if httpPromptCacheWantOwner04383(shape, stage) == "account-b" {
				want = "second"
			}
			if (shape == "nonfinite-unrelated" || shape == "overdeep-unrelated") && stage == "seed-second" {
				// A serde_json parse error suppresses the response ID as well
				// as the cached-token observation, while HTTP 200 is passed on.
				want = ""
			}
			if err != nil || owner != want {
				ready = false
				break
			}
		}
		if ready {
			break
		}
		if time.Now().After(deadline) {
			return errors.New("prompt cache durable response ownership did not match route")
		}
		time.Sleep(20 * time.Millisecond)
	}
	fmt.Printf("%s prompt-cache %s: authenticated Ready accounts; four exact attempts; fresh owner=%s; expected durable response owners verified\n", p.name, shape, httpPromptCacheWantOwner04383(shape, "fresh"))
	return nil
}

func verifyHTTPPromptCache04383(shape string, result httpPromptCacheResult04383) error {
	stages := []string{"seed-main", "seed-second", "restore-main", "fresh"}
	if result.invalid || len(result.attempts) != len(stages) || len(result.replies) != len(stages) {
		return fmt.Errorf("prompt-cache source invalid or wrong attempt count: %+v", result)
	}
	for index, stage := range stages {
		if reply := result.replies[index]; reply.status != http.StatusOK || reply.body != httpPromptCacheReply04383(shape, stage) {
			return fmt.Errorf("prompt-cache %s client response changed: %+v", stage, reply)
		}
		attempt := result.attempts[index]
		if !attempt.ready || attempt.owner != httpPromptCacheWantOwner04383(shape, stage) ||
			attempt.body != httpPromptCacheRequest04383(stage) || attempt.turn != "" || attempt.metadata != "" || attempt.trace != "" {
			return fmt.Errorf("prompt-cache %s changed routing/wire: %+v", stage, attempt)
		}
	}
	return nil
}
