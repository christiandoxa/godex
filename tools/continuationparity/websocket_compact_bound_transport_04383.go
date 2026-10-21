package main

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

// Prodex 0.438.4 treats a Compact-origin turn alias as a turn-state profile
// when full request history is independently reconstructable. On a precommit
// EOF, the original second-owned attempt can replay once on authenticated
// Ready main with its stale turn token scrubbed. The Compact alias remains on
// second; neither the failure nor the replay may steal that durable binding.
func runCompactBoundTransportCase04383(root string, p product) error {
	if err := prepareProduct(root, &p); err != nil {
		return err
	}
	if err := seedTransportAlternate04383(p, "compact-lineage"); err != nil {
		return err
	}
	alias, err := readCompactLineageOwner04383(p)
	if err != nil || alias != "second" {
		return fmt.Errorf("no verified second-owned Compact turn: owner=%q err=%v", alias, err)
	}
	p.env = append(p.env, "PRODEX_RUNTIME_LOG_RECORD=true")
	backend := newTransportBackend04383("compact-bound-transport")
	defer backend.server.Close()
	endpoint, stop, err := startTransportLaunch04383(p, backend.server.URL+"/backend-api")
	if err != nil {
		return err
	}
	defer stop()
	client := &http.Client{Timeout: 5 * time.Second}
	for _, previous := range []string{"resp-ready-b", "resp-main"} {
		warmup, err := post(client, endpoint+"/responses", fmt.Sprintf(`{"previous_response_id":%q,"input":[]}`, previous))
		if err != nil || warmup.status != http.StatusOK {
			return fmt.Errorf("strict Compact warmup %s: status=%d err=%v", previous, warmup.status, err)
		}
	}
	readyDeadline := time.Now().Add(5 * time.Second)
	for !transportProfilesReady04383(p, backend) {
		if time.Now().After(readyDeadline) {
			return errors.New("strict Compact fixture did not establish both applied Ready quota receipts")
		}
		time.Sleep(10 * time.Millisecond)
	}
	connection, reader, err := wsConnectHeaders04383(endpoint+"/responses", "")
	if err != nil {
		return err
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(12 * time.Second))
	if err := websocketframe.WriteFrame(connection, 1, []byte(compactLineageMessage04383), true); err != nil {
		return err
	}
	frames := make([]string, 0, 2)
	for len(frames) < 2 {
		opcode, payload, _, err := readWebSocketLivenessFrame04383(connection, reader)
		if err != nil {
			backend.mu.Lock()
			handshakes := append([]string(nil), backend.handshakes...)
			attempts := append([]transportAttempt04383(nil), backend.attempts...)
			backend.mu.Unlock()
			return fmt.Errorf("Compact precommit client: %w frames=%q handshakes=%v attempts=%+v", err, frames, handshakes, attempts)
		}
		if opcode != 1 {
			return fmt.Errorf("Compact precommit returned unexpected opcode %d", opcode)
		}
		frames = append(frames, string(payload))
	}
	backend.mu.Lock()
	handshakes := append([]string(nil), backend.handshakes...)
	attempts := append([]transportAttempt04383(nil), backend.attempts...)
	invalid := backend.invalid
	backend.mu.Unlock()
	if err := verifyCompactBoundTransport04383(handshakes, attempts, invalid, frames); err != nil {
		return err
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		owners, err := readTransportOwners04383(p)
		alias, aliasErr := readCompactLineageOwner04383(p)
		if err == nil && aliasErr == nil && alias == "second" &&
			owners["turn"] == "main" && owners["unrelated"] == "main" && owners["session"] == "main" && owners["response"] == "main" {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("strict Compact precommit lost durable owner: owners=%+v alias=%q errors=%v/%v", owners, alias, err, aliasErr)
		}
		time.Sleep(20 * time.Millisecond)
	}
	fmt.Printf("%s Compact precommit: authenticated Ready second-to-main full-history replay; stale token scrubbed; two exact attempts; visible completion; original alias retained\n", p.name)
	return nil
}

func verifyCompactBoundTransport04383(handshakes []string, attempts []transportAttempt04383, invalid bool, frames []string) error {
	clean := strings.Replace(compactLineageMessage04383, `"x-codex-turn-state":"compact-turn",`, "", 1)
	if invalid || !reflect.DeepEqual(handshakes, []string{"account-b", "account-a"}) || len(attempts) != 2 ||
		!attempts[0].ready || attempts[0].owner != "account-b" || attempts[0].turn != "compact-turn" ||
		!sameTransportJSON04383(attempts[0].body, compactLineageMessage04383) ||
		!attempts[1].ready || attempts[1].owner != "account-a" || attempts[1].turn != "" ||
		!sameTransportJSON04383(attempts[1].body, clean) ||
		!reflect.DeepEqual(frames, []string{transportDelta04383, transportCompleted04383}) {
		return fmt.Errorf("Compact precommit changed full-history replay/owner/frames: handshakes=%v attempts=%+v invalid=%t frames=%q", handshakes, attempts, invalid, frames)
	}
	return nil
}
