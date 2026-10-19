package main

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

func TestProdex04383HTTPTransportWaitsForDurableOwnersBeforeChildRelease(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := product{name: "prodex", state: t.TempDir()}
		path := filepath.Join(p.state, "runtime-continuations.json")
		snapshot := func(session string) map[string]any {
			return map[string]any{"value": map[string]any{
				"session_id_bindings":       map[string]any{"sess-transport": map[string]string{"profile_name": session}},
				"response_profile_bindings": map[string]any{"resp-main": map[string]string{"profile_name": "main"}},
			}}
		}
		if err := writeJSONFile(path, snapshot("main")); err != nil {
			t.Fatal(err)
		}
		result := httpTransportResult04383{
			reply: exchange{status: 200, body: compactReply04383},
			attempts: []transportAttempt04383{
				{owner: "account-a", turn: "turn-a", body: httpTransportBody04383("with-session"), ready: true},
				{owner: "account-b", turn: "turn-a", body: httpTransportBody04383("with-session"), ready: true},
			},
		}
		observed := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			<-observed
			<-time.After(150 * time.Millisecond)
			done <- writeJSONFile(path, snapshot("__prodex_hard_binding_conflict__"))
		}()
		var first sync.Once
		verify := func(scenario string, value httpTransportResult04383) error {
			err := verifyHTTPCompact04383(scenario, value)
			if err != nil {
				first.Do(func() { close(observed) })
			}
			return err
		}
		err := waitHTTPTransportDurable04383(p, "with-session", &result, verify)
		first.Do(func() { close(observed) })
		if writeErr := <-done; writeErr != nil {
			t.Fatal(writeErr)
		}
		if err != nil || result.owners["session"] != "__prodex_hard_binding_conflict__" {
			t.Fatalf("child release did not wait for the durable session conflict: owners=%v err=%v", result.owners, err)
		}
		if err := writeJSONFile(path, snapshot("main")); err != nil {
			t.Fatal(err)
		}
		started := time.Now()
		err = waitHTTPTransportDurable04383(p, "with-session", &result, verifyHTTPCompact04383)
		if err == nil || !strings.Contains(err.Error(), "durable HTTP transport state did not settle") || time.Since(started) > 3*time.Second {
			t.Fatalf("unsettled durable state was accepted or unbounded: elapsed=%s err=%v", time.Since(started), err)
		}
	})
}
