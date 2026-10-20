package main

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

// A Compact request establishes a separate durable turn lineage. The tagged
// 0.438.3 WebSocket selector honors that lineage even though its request's
// quota route remains WebSocket. The unbound control must stay on main.
const compactLineageMessage04383 = `{"type":"response.create","model":"gpt-6","x-codex-turn-state":"compact-turn","input":[{"type":"message","role":"user","content":"first"},{"type":"message","role":"assistant","content":"answer"},{"type":"message","role":"user","content":"followup"}]}`
const compactPreviousConflictMessage04383 = `{"type":"response.create","model":"gpt-6","previous_response_id":"resp-main","x-codex-turn-state":"compact-turn","input":[{"type":"message","role":"user","content":"first"},{"type":"message","role":"assistant","content":"answer"},{"type":"message","role":"user","content":"followup"}]}`
const compactTurnConflictTerminal04383 = `{"error":{"code":"service_unavailable","message":"Runtime proxy could not secure a healthy upstream profile before the pre-commit retry budget was exhausted. Retry the request."},"status":503,"type":"error"}`
const compactShadowNullMessage04383 = `{"type":"response.create","model":"gpt-6","x-codex-turn-state":null,"client_metadata":{"x-codex-turn-state":"compact-turn"},"input":[{"type":"message","role":"user","content":"first"},{"type":"message","role":"assistant","content":"answer"},{"type":"message","role":"user","content":"followup"}]}`
const compactShadowBlankMessage04383 = `{"type":"response.create","model":"gpt-6","x-codex-turn-state":" ","client_metadata":{"x-codex-turn-state":"compact-turn"},"input":[{"type":"message","role":"user","content":"first"},{"type":"message","role":"assistant","content":"answer"},{"type":"message","role":"user","content":"followup"}]}`
const nestedSessionMessage04383 = `{"type":"response.create","model":"gpt-6","client_metadata":{"session_id":"shadow-session"},"input":[{"type":"message","role":"user","content":"first"},{"type":"message","role":"assistant","content":"answer"},{"type":"message","role":"user","content":"followup"}]}`
const shadowSessionNullMessage04383 = `{"type":"response.create","model":"gpt-6","session_id":null,"client_metadata":{"session_id":"shadow-session"},"input":[{"type":"message","role":"user","content":"first"},{"type":"message","role":"assistant","content":"answer"},{"type":"message","role":"user","content":"followup"}]}`
const shadowSessionBlankMessage04383 = `{"type":"response.create","model":"gpt-6","session_id":" ","client_metadata":{"session_id":"shadow-session"},"input":[{"type":"message","role":"user","content":"first"},{"type":"message","role":"assistant","content":"answer"},{"type":"message","role":"user","content":"followup"}]}`
const compactSessionHeaderMessage04383 = `{"type":"response.create","model":"gpt-6","input":[{"type":"message","role":"user","content":"first"},{"type":"message","role":"assistant","content":"answer"},{"type":"message","role":"user","content":"followup"}]}`
const compactSessionBodyMessage04383 = `{"type":"response.create","model":"gpt-6","session_id":"session-alias","input":[{"type":"message","role":"user","content":"first"},{"type":"message","role":"assistant","content":"answer"},{"type":"message","role":"user","content":"followup"}]}`

func runWebSocketCompactLineageProduct04383(root string, p product) error {
	for _, scenario := range []string{"compact-lineage", "compact-turn-conflict", "compact-previous-conflict", "compact-unbound", "compact-shadow-null", "compact-shadow-blank", "session-nested", "session-shadow-null", "session-shadow-blank", "compact-session-header", "compact-session-unbound", "compact-session-body", "compact-session-conflict", "compact-produced", "compact-produced-long-turn"} {
		if err := runCompactLineageCase04383(filepath.Join(root, scenario), p, scenario); err != nil {
			return fmt.Errorf("%s: %w", scenario, err)
		}
	}
	if err := runCompactBoundTransportCase04383(filepath.Join(root, "compact-bound-transport"), p); err != nil {
		return fmt.Errorf("compact-bound-transport: %w", err)
	}
	return nil
}

func runCompactLineageCase04383(root string, p product, scenario string) error {
	if err := prepareProduct(root, &p); err != nil {
		return err
	}
	if err := seedTransportAlternate04383(p, scenario); err != nil {
		return err
	}
	initial, err := readTransportOwners04383(p)
	if err != nil {
		return err
	}
	initialLineage, err := readCompactLineageOwner04383(p)
	if err != nil {
		return err
	}
	wantOwner := "second"
	if scenario == "compact-unbound" || scenario == "compact-produced-long-turn" || (scenario == "compact-session-unbound" || scenario == "compact-session-conflict") || strings.HasPrefix(scenario, "compact-shadow-") || strings.HasPrefix(scenario, "session-shadow-") {
		wantOwner = "main"
	}
	wantLineage := ""
	if scenario == "compact-lineage" || scenario == "compact-turn-conflict" || scenario == "compact-previous-conflict" || strings.HasPrefix(scenario, "compact-shadow-") {
		wantLineage = "second"
	}
	if initialLineage != wantLineage || initial["turn"] != "main" || initial["unrelated"] != "main" || initial["session"] != "main" {
		return fmt.Errorf("invalid initial Compact lineage fixture: lineage=%q main_turn=%q main_response=%q session=%q",
			initialLineage, initial["turn"], initial["unrelated"], initial["session"])
	}
	if scenario == "compact-turn-conflict" {
		owner, err := readCompactBindingOwner04383(p, "turn", "compact-turn")
		if err != nil || owner != "main" {
			return fmt.Errorf("invalid direct Compact turn conflict fixture: owner=%q err=%v", owner, err)
		}
	}
	if strings.HasPrefix(scenario, "compact-session-") {
		aliasOwner, aliasErr := readCompactBindingOwner04383(p, "session", "__compact_session__:session-alias")
		plainOwner, plainErr := readCompactBindingOwner04383(p, "session", "session-alias")
		wantAlias := "second"
		if scenario == "compact-session-unbound" {
			wantAlias = ""
		}
		wantPlain := ""
		if scenario == "compact-session-conflict" {
			wantPlain = "main"
		}
		if aliasErr != nil || plainErr != nil || aliasOwner != wantAlias || plainOwner != wantPlain {
			return fmt.Errorf("invalid Compact session alias fixture: alias=%q plain=%q errors=%v/%v", aliasOwner, plainOwner, aliasErr, plainErr)
		}
	}
	if strings.HasPrefix(scenario, "session-") {
		owner, err := readCompactBindingOwner04383(p, "session", "shadow-session")
		if err != nil || owner != "second" {
			return fmt.Errorf("invalid source-verified nested session fixture: owner=%q err=%v", owner, err)
		}
	}
	p.env = append(p.env, "PRODEX_RUNTIME_LOG_RECORD=true")
	backend := newTransportBackend04383(scenario)
	defer backend.server.Close()
	endpoint, stop, err := startTransportLaunch04383(p, backend.server.URL+"/backend-api")
	if err != nil {
		return err
	}
	defer stop()
	client := &http.Client{Timeout: 5 * time.Second}
	// For source-derived production, leave second active so its HTTP
	// Compact commit creates the lineage, then return current to main.
	// Synthetic controls warm second first and main last directly.
	warmups := []string{"resp-ready-b", "resp-main"}
	if strings.HasPrefix(scenario, "compact-produced") {
		warmups = []string{"resp-main", "resp-ready-b"}
	}
	for _, previous := range warmups {
		warmup, err := post(client, endpoint+"/responses", fmt.Sprintf(`{"previous_response_id":%q,"input":[]}`, previous))
		if err != nil || warmup.status != http.StatusOK {
			return fmt.Errorf("Compact owner warmup %s: status=%d err=%v", previous, warmup.status, err)
		}
	}
	readyDeadline := time.Now().Add(5 * time.Second)
	for !transportProfilesReady04383(p, backend) {
		if time.Now().After(readyDeadline) {
			return errors.New("Compact lineage fixture has no applied authenticated Ready quota receipts")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if strings.HasPrefix(scenario, "compact-produced") {
		// Use a previously verified second-owned response as the Compact
		// producer's independent hard owner. This avoids relying on the
		// products' different fresh-pool/active-profile heuristics.
		response, err := post(client, endpoint+"/responses/compact", `{"previous_response_id":"resp-ready-b","model":"gpt-6","session_id":"compact-produced-session","input":[]}`)
		if err != nil || response.status != http.StatusOK || !strings.Contains(response.body, "resp-compact-produced") {
			return fmt.Errorf("live HTTP Compact producer: status=%d err=%v response=%s", response.status, err, response.body)
		}
		backend.mu.Lock()
		compactOwners := append([]string(nil), backend.compactOwners...)
		invalid := backend.invalid
		backend.mu.Unlock()
		if invalid || !reflect.DeepEqual(compactOwners, []string{"account-b"}) {
			return fmt.Errorf("HTTP Compact producer changed verified owner: attempts=%v invalid=%t", compactOwners, invalid)
		}
		producedDeadline := time.Now().Add(3 * time.Second)
		wantProducedLineage := "second"
		if scenario == "compact-produced-long-turn" {
			wantProducedLineage = ""
		}
		for {
			lineage, lineageErr := readCompactLineageOwner04383(p)
			longAlias, longAliasErr := readCompactBindingOwner04383(p, "turn", "__compact_turn_state__:"+strings.Repeat("L", 1025))
			session, sessionErr := readCompactSessionOwner04383(p)
			direct, directErr := readCompactDirectSessionOwner04383(p)
			if lineageErr == nil && longAliasErr == nil && sessionErr == nil && directErr == nil &&
				lineage == wantProducedLineage && longAlias == "" && session == "second" && direct == "second" {
				break
			}
			if time.Now().After(producedDeadline) {
				return fmt.Errorf("successful HTTP Compact produced wrong bounded durable lineage: short=%q long=%q session=%q direct=%q errors=%v/%v/%v/%v", lineage, longAlias, session, direct, lineageErr, longAliasErr, sessionErr, directErr)
			}
			time.Sleep(20 * time.Millisecond)
		}
		// Only the Compact-produced lineage may justify selecting second.
		// Restore main as the current account before the WebSocket request.
		main, err := post(client, endpoint+"/responses", `{"previous_response_id":"resp-main","input":[]}`)
		if err != nil || main.status != http.StatusOK {
			return fmt.Errorf("restore main after Compact production: status=%d err=%v", main.status, err)
		}
		wantLineage = wantProducedLineage
	}
	wsHeaders := ""
	if scenario == "compact-session-header" || scenario == "compact-session-unbound" || scenario == "compact-session-conflict" {
		wsHeaders = "session_id: session-alias\r\n"
	}
	connection, reader, err := wsConnectHeaders04383(endpoint+"/responses", wsHeaders)
	if err != nil {
		return err
	}
	defer connection.Close()
	deadlineSeconds := 12 * time.Second
	if scenario == "compact-turn-conflict" || scenario == "compact-previous-conflict" {
		deadlineSeconds = 36 * time.Second
	}
	_ = connection.SetDeadline(time.Now().Add(deadlineSeconds))
	message := compactLineageMessage04383
	if scenario == "compact-shadow-null" {
		message = compactShadowNullMessage04383
	} else if scenario == "compact-shadow-blank" {
		message = compactShadowBlankMessage04383
	} else if scenario == "compact-previous-conflict" {
		message = compactPreviousConflictMessage04383
	} else if scenario == "session-nested" {
		message = nestedSessionMessage04383
	} else if scenario == "session-shadow-null" {
		message = shadowSessionNullMessage04383
	} else if scenario == "session-shadow-blank" {
		message = shadowSessionBlankMessage04383
	} else if scenario == "compact-session-header" || scenario == "compact-session-unbound" || scenario == "compact-session-conflict" {
		message = compactSessionHeaderMessage04383
	} else if scenario == "compact-session-body" {
		message = compactSessionBodyMessage04383
	}
	if err := websocketframe.WriteFrame(connection, 1, []byte(message), true); err != nil {
		return err
	}
	frames := make([]string, 0, 2)
	for len(frames) < 2 {
		opcode, payload, _, err := readWebSocketLivenessFrame04383(connection, reader)
		if err != nil {
			return fmt.Errorf("Compact lineage client: %w; frames=%q", err, frames)
		}
		if opcode != 1 {
			return fmt.Errorf("unexpected Compact lineage client opcode %d", opcode)
		}
		frames = append(frames, string(payload))
		if scenario == "compact-turn-conflict" || scenario == "compact-previous-conflict" {
			backend.mu.Lock()
			check := verifyCompactTurnConflict04383(backend.handshakes, backend.attempts, frames, backend.invalid)
			backend.mu.Unlock()
			if check != nil {
				return check
			}
			aliasOwner, aliasErr := readCompactLineageOwner04383(p)
			directOwner, directErr := readCompactBindingOwner04383(p, "turn", "compact-turn")
			owners, ownersErr := readTransportOwners04383(p)
			wantDirect := "main"
			if scenario == "compact-previous-conflict" {
				wantDirect = ""
			}
			if aliasErr != nil || directErr != nil || ownersErr != nil ||
				aliasOwner != "second" || directOwner != wantDirect ||
				owners["turn"] != "main" || owners["unrelated"] != "main" ||
				owners["response"] != "" || owners["session"] != "main" {
				return fmt.Errorf("conflicting Compact turn owner mutated: direct=%q lineage=%q owners=%v errors=%v/%v/%v", directOwner, aliasOwner, owners, directErr, aliasErr, ownersErr)
			}
			fmt.Printf("%s %s: verified conflicting owners; 503 service_unavailable; zero WebSocket upstream attempts; durable bindings unchanged\n", p.name, scenario)
			return nil
		}
	}
	backend.mu.Lock()
	header := "compact-turn"
	if strings.HasPrefix(scenario, "compact-shadow-") || strings.HasPrefix(scenario, "session-") || strings.HasPrefix(scenario, "compact-session-") {
		header = ""
	}
	err = verifyCompactLineageWire04383(wantOwner, message, header, backend.handshakes, backend.attempts, frames, backend.invalid)
	backend.mu.Unlock()
	if err != nil {
		return err
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		owners, err := readTransportOwners04383(p)
		lineage, lineageErr := readCompactLineageOwner04383(p)
		sessionOK := true
		if strings.HasPrefix(scenario, "compact-session-") {
			aliasOwner, aliasErr := readCompactBindingOwner04383(p, "session", "__compact_session__:session-alias")
			plainOwner, plainErr := readCompactBindingOwner04383(p, "session", "session-alias")
			wantAlias := "second"
			if scenario == "compact-session-unbound" {
				wantAlias = ""
			}
			// Tagged Prodex commits a direct session owner on successful
			// WebSocket output, even if routing began from a typed Compact
			// alias. The initial state above still requires it to be absent
			// except in the conflicting-direct-owner control.
			sessionOK = aliasErr == nil && plainErr == nil &&
				aliasOwner == wantAlias && plainOwner == wantOwner
		}
		if strings.HasPrefix(scenario, "session-") {
			shadowOwner, shadowErr := readCompactBindingOwner04383(p, "session", "shadow-session")
			sessionOK = shadowErr == nil && shadowOwner == "second"
		}
		if strings.HasPrefix(scenario, "compact-produced") {
			aliasSession, aliasErr := readCompactSessionOwner04383(p)
			directSession, directErr := readCompactDirectSessionOwner04383(p)
			longAlias, longAliasErr := readCompactBindingOwner04383(p, "turn", "__compact_turn_state__:"+strings.Repeat("L", 1025))
			sessionOK = aliasErr == nil && directErr == nil && longAliasErr == nil &&
				aliasSession == "second" && directSession == "second" && longAlias == ""
		}
		if err == nil && lineageErr == nil && sessionOK && lineage == wantLineage && owners["response"] == wantOwner &&
			owners["turn"] == "main" && owners["session"] == "main" && owners["unrelated"] == "main" {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("Compact lineage durable owner projection: %+v lineage=%q session_ok=%t errors=%v/%v", owners, lineage, sessionOK, err, lineageErr)
		}
		time.Sleep(20 * time.Millisecond)
	}
	fmt.Printf("%s %s: applied Ready profiles; exact full-history input; one WebSocket handshake/attempt on %s; unchanged Compact lineage and durable response owner\n", p.name, scenario, wantOwner)
	return nil
}

func verifyCompactTurnConflict04383(handshakes []string, attempts []transportAttempt04383, frames []string, invalid bool) error {
	if invalid || len(handshakes) != 0 || len(attempts) != 0 ||
		!reflect.DeepEqual(frames, []string{compactTurnConflictTerminal04383}) {
		return fmt.Errorf("unsafe conflicting Compact turn ownership: handshakes=%v attempts=%+v invalid=%t frames=%q", handshakes, attempts, invalid, frames)
	}
	return nil
}

func verifyCompactLineage04383(wantOwner string, handshakes []string, attempts []transportAttempt04383, frames []string, invalid bool) error {
	return verifyCompactLineageWire04383(wantOwner, compactLineageMessage04383, "compact-turn", handshakes, attempts, frames, invalid)
}

func verifyCompactLineageWire04383(wantOwner, wantBody, wantTurn string, handshakes []string, attempts []transportAttempt04383, frames []string, invalid bool) error {
	wantAccount := map[string]string{"main": "account-a", "second": "account-b"}[wantOwner]
	if invalid || wantAccount == "" || !reflect.DeepEqual(handshakes, []string{wantAccount}) || len(attempts) != 1 ||
		!attempts[0].ready || attempts[0].owner != wantAccount || attempts[0].turn != wantTurn ||
		!sameTransportJSON04383(attempts[0].body, wantBody) ||
		!reflect.DeepEqual(frames, []string{transportDelta04383, transportCompleted04383}) {
		return fmt.Errorf("Compact lineage owner/provenance/frame mismatch: want=%s handshakes=%v attempts=%+v invalid=%t frames=%q", wantAccount, handshakes, attempts, invalid, frames)
	}
	return nil
}
