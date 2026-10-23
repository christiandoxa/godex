package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const nativeTurnMetadata04390 = `{"request_kind":"memory","sandbox_mode":"read-only","thread_source":"memory_consolidation","future_metadata":{"preserve":true},"root_turn_id":"root-0164","reviewed_patch_label":"native-merged-metadata"}`
const nativeSession04390 = "independent-session-0164"
const nativeMetadataReply04390 = `{"id":"resp-native-metadata-0164","object":"response","output":[]}`

func nativeMetadataRequest04390(shape string) string {
	cache := ""
	if shape == "custom" {
		cache = `,"prompt_cache_key":"custom-cache-key-not-session-0164"`
	}
	return `{"model":"gpt-6","input":[{"type":"message","role":"developer","content":[{"type":"input_text","text":"Consolidate detached memory."}]}],"client_metadata":{"x-codex-turn-metadata":` + strconv.Quote(nativeTurnMetadata04390) + `,"request_origin":"detached_memory","future_client_field":{"preserve":true}},"future_body_field":["preserve",9007199254740993]` + cache + `}`
}

func (b *transportBackend04383) serveNativeMetadata04390(w http.ResponseWriter, r *http.Request, body string) {
	b.mu.Lock()
	prior := len(b.attempts)
	b.attempts = append(b.attempts, transportAttempt04383{
		owner: r.Header.Get("ChatGPT-Account-Id"), body: body,
		metadata: r.Header.Get("x-codex-turn-metadata"), session: r.Header.Get("session_id"),
		turn: r.Header.Get("x-codex-turn-state"), trace: r.Header.Get("thread-id"),
		userAgent: r.Header.Get("User-Agent"), ready: b.receipts["account-a"] > 0 && (b.scenario != "http-native-metadata-rotation" || b.receipts["account-b"] > 0),
	})
	b.mu.Unlock()
	if b.scenario == "http-native-metadata-rotation" && prior == 0 {
		writeJSON(w, http.StatusTooManyRequests, `{"error":{"code":"insufficient_quota","message":"synthetic quota"}}`)
		return
	}
	writeJSON(w, http.StatusOK, nativeMetadataReply04390)
}

func runHTTPNativeMetadataProduct04390(root string, p product) error {
	for _, shape := range []string{"omitted", "custom", "rotation"} {
		if err := runHTTPNativeMetadataCase04390(filepath.Join(root, shape), p, shape); err != nil {
			return fmt.Errorf("native metadata %s: %w", shape, err)
		}
	}
	return nil
}

func runHTTPNativeMetadataCase04390(root string, p product, shape string) error {
	if err := prepareProduct(root, &p); err != nil {
		return err
	}
	if err := seedTransportAlternate04383(p, "http-native-metadata-"+shape); err != nil {
		return err
	}
	p.env = append(p.env, "PRODEX_RUNTIME_LOG_RECORD=true")
	b := newTransportBackend04383("http-native-metadata-" + shape)
	defer b.server.Close()
	// The tagged custom-cache fixture fixes one profile; placement policy is separate.
	var endpoint string
	var stop func()
	var err error
	if shape == "rotation" {
		endpoint, stop, err = startTransportLaunch04383(p, b.server.URL+"/backend-api")
	} else {
		var command *exec.Cmd
		endpoint, command, err = startGateway(p, b.server.URL+"/backend-api")
		stop = func() { stopGateway(command) }
	}
	if err != nil {
		return err
	}
	defer stop()
	client := &http.Client{Timeout: 8 * time.Second}
	previousIDs := []string{"resp-main"}
	if shape == "rotation" {
		previousIDs = []string{"resp-ready-b", "resp-main"}
	}
	for _, previous := range previousIDs {
		warmup, err := post(client, endpoint+"/responses", fmt.Sprintf(`{"previous_response_id":%q,"input":[]}`, previous))
		if err != nil || warmup.status != http.StatusOK {
			return fmt.Errorf("native metadata warmup %s: status=%d err=%v", previous, warmup.status, err)
		}
	}
	until := time.Now().Add(5 * time.Second)
	for {
		b.mu.Lock()
		ready := b.receipts["account-a"] > 0 && !b.invalid
		b.mu.Unlock()
		if shape == "rotation" {
			ready = transportProfilesReady04383(p, b)
		}
		if ready {
			break
		}
		if time.Now().After(until) {
			return errors.New("native metadata requires two authenticated Ready profiles")
		}
		time.Sleep(10 * time.Millisecond)
	}
	request, err := http.NewRequest(http.MethodPost, endpoint+"/responses", strings.NewReader(nativeMetadataRequest04390(shape)))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "")
	request.Header.Set("session_id", nativeSession04390)
	request.Header.Set("x-codex-turn-metadata", nativeTurnMetadata04390)
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	data, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	closeErr := response.Body.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return err
	}
	b.mu.Lock()
	result := httpSoftConflictResult04383{reply: exchange{status: response.StatusCode, body: string(data)},
		attempts: append([]transportAttempt04383(nil), b.attempts...), invalid: b.invalid}
	b.mu.Unlock()
	if err := verifyNativeMetadata04390(shape, result); err != nil {
		return err
	}
	wantOwner := "main"
	if shape == "rotation" {
		wantOwner = "second"
	}
	until = time.Now().Add(3 * time.Second)
	for {
		owner, ownerErr := readHTTPResponseOwner04383(p, "resp-native-metadata-0164")
		main, mainErr := readHTTPResponseOwner04383(p, "resp-main")
		second, secondErr := readHTTPResponseOwner04383(p, "resp-ready-b")
		if ownerErr == nil && mainErr == nil && secondErr == nil && owner == wantOwner && main == "main" && second == "second" {
			break
		}
		if time.Now().After(until) {
			return fmt.Errorf("native metadata durable owners changed: new=%q main=%q second=%q errors=%v/%v/%v", owner, main, second, ownerErr, mainErr, secondErr)
		}
		time.Sleep(20 * time.Millisecond)
	}
	fmt.Printf("%s native metadata %s: %d authenticated exact attempts; cache/session/unknown fields preserved; durable owner=%s\n", p.name, shape, len(result.attempts), wantOwner)
	return nil
}

func verifyNativeMetadata04390(shape string, result httpSoftConflictResult04383) error {
	want := 1
	if shape == "rotation" {
		want = 2
	} else if shape != "custom" && shape != "omitted" {
		return errors.New("unknown native metadata shape")
	}
	if result.invalid || len(result.attempts) != want || result.reply.status != http.StatusOK || result.reply.body != nativeMetadataReply04390 {
		return errors.New("native metadata authentication, attempt count or response changed")
	}
	for index, attempt := range result.attempts {
		owner := "account-a"
		if index == 1 {
			owner = "account-b"
		}
		if attempt.owner != owner || !attempt.ready || attempt.body != nativeMetadataRequest04390(shape) || attempt.metadata != nativeTurnMetadata04390 || attempt.session != nativeSession04390 || attempt.turn != "" || attempt.trace != "" || attempt.userAgent != "" {
			return fmt.Errorf("native metadata attempt %d changed: owner=%s ready=%t body_equal=%t metadata_equal=%t session_equal=%t turn_empty=%t trace_empty=%t user_agent_empty=%t", index, attempt.owner, attempt.ready, attempt.body == nativeMetadataRequest04390(shape), attempt.metadata == nativeTurnMetadata04390, attempt.session == nativeSession04390, attempt.turn == "", attempt.trace == "", attempt.userAgent == "")
		}
	}
	return nil
}
