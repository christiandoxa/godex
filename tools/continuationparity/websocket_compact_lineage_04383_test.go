package main

import (
	"strings"
	"testing"
)

func TestProdex04383WebSocketCompactLineageOracleRejectsMutations(t *testing.T) {
	for _, expected := range []string{"main", "second"} {
		owner := "account-a"
		if expected == "second" {
			owner = "account-b"
		}
		attempt := transportAttempt04383{owner: owner, turn: "compact-turn", ready: true, body: compactLineageMessage04383}
		hello := []string{owner}
		frames := []string{transportDelta04383, transportCompleted04383}
		if err := verifyCompactLineage04383(expected, hello, []transportAttempt04383{attempt}, frames, false); err != nil {
			t.Fatalf("valid %s Compact lineage failed: %v", expected, err)
		}
		for _, mutation := range []struct {
			name     string
			hello    []string
			attempts []transportAttempt04383
			frames   []string
			invalid  bool
		}{
			{"missing readiness", hello, []transportAttempt04383{{owner: owner, body: compactLineageMessage04383}}, frames, false},
			{"wrong owner", []string{"account-a", "account-b"}, []transportAttempt04383{attempt}, frames, false},
			{"stale turn header", hello, []transportAttempt04383{{owner: owner, turn: "foreign-turn", ready: true, body: compactLineageMessage04383}}, frames, false},
			{"changed opaque history", hello, []transportAttempt04383{{owner: owner, turn: "compact-turn", ready: true, body: strings.ReplaceAll(compactLineageMessage04383, `"content":"answer"`, `"content":"changed"`)}}, frames, false},
			{"duplicated model request", hello, []transportAttempt04383{attempt, attempt}, frames, false},
			{"changed client output", hello, []transportAttempt04383{attempt}, []string{`{"type":"response.output_text.delta","delta":"wrong"}`, transportCompleted04383}, false},
			{"lost terminal", hello, []transportAttempt04383{attempt}, []string{transportDelta04383}, false},
			{"invalid credentials", hello, []transportAttempt04383{attempt}, frames, true},
		} {
			t.Run(expected+"/"+mutation.name, func(t *testing.T) {
				if err := verifyCompactLineage04383(expected, mutation.hello, mutation.attempts, mutation.frames, mutation.invalid); err == nil {
					t.Fatalf("Compact lineage oracle accepted %s mutation", mutation.name)
				}
			})
		}
	}
}

func TestProdex04383WebSocketShadowedDirectMetadataOracleRejectsMutations(t *testing.T) {
	for name, body := range map[string]string{
		"null":  compactShadowNullMessage04383,
		"blank": compactShadowBlankMessage04383,
	} {
		t.Run(name, func(t *testing.T) {
			base := transportAttempt04383{owner: "account-a", body: body, ready: true}
			hello := []string{"account-a"}
			frames := []string{transportDelta04383, transportCompleted04383}
			if err := verifyCompactLineageWire04383("main", body, "", hello, []transportAttempt04383{base}, frames, false); err != nil {
				t.Fatal(err)
			}
			for _, mutation := range []struct {
				name     string
				attempts []transportAttempt04383
				frames   []string
			}{
				{"incorrect nested owner", []transportAttempt04383{{owner: "account-b", body: body, ready: true}}, frames},
				{"leaked nested token", []transportAttempt04383{{owner: "account-a", turn: "compact-turn", body: body, ready: true}}, frames},
				{"lost Ready receipt", []transportAttempt04383{{owner: "account-a", body: body}}, frames},
				{"damaged user request", []transportAttempt04383{{owner: "account-a", body: strings.Replace(body, `"content":"answer"`, `"content":"lost"`, 1), ready: true}}, frames},
				{"duplicated request", []transportAttempt04383{base, base}, frames},
				{"wrong terminal", []transportAttempt04383{base}, []string{transportDelta04383, `{"type":"response.completed","response":{"id":"wrong"}}`}},
			} {
				t.Run(mutation.name, func(t *testing.T) {
					if err := verifyCompactLineageWire04383("main", body, "", hello, mutation.attempts, mutation.frames, false); err == nil {
						t.Fatalf("shadowed metadata material mutation escaped: %s", mutation.name)
					}
				})
			}
		})
	}
}

func TestProdex04383WebSocketDirectSessionShadowsNestedOracleRejectsMutations(t *testing.T) {
	for _, tc := range []struct {
		name, body, owner, account string
	}{
		{"nested-control", nestedSessionMessage04383, "second", "account-b"},
		{"direct-null", shadowSessionNullMessage04383, "main", "account-a"},
		{"direct-whitespace", shadowSessionBlankMessage04383, "main", "account-a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := transportAttempt04383{owner: tc.account, ready: true, body: tc.body}
			frames := []string{transportDelta04383, transportCompleted04383}
			if err := verifyCompactLineageWire04383(tc.owner, tc.body, "", []string{tc.account}, []transportAttempt04383{base}, frames, false); err != nil {
				t.Fatalf("valid session-provenance control rejected: %v", err)
			}
			for _, mutation := range []struct {
				name     string
				attempts []transportAttempt04383
				frames   []string
			}{
				{"leaked turn header", []transportAttempt04383{{owner: tc.account, turn: "foreign-turn", ready: true, body: tc.body}}, frames},
				{"changed history", []transportAttempt04383{{owner: tc.account, ready: true, body: strings.Replace(tc.body, `"content":"answer"`, `"content":"changed"`, 1)}}, frames},
				{"replayed model", []transportAttempt04383{base, base}, frames},
				{"missing Ready evidence", []transportAttempt04383{{owner: tc.account, body: tc.body}}, frames},
				{"wrong terminal identity", []transportAttempt04383{base}, []string{transportDelta04383, `{"type":"response.completed","response":{"id":"incorrect"}}`}},
			} {
				t.Run(mutation.name, func(t *testing.T) {
					if err := verifyCompactLineageWire04383(tc.owner, tc.body, "", []string{tc.account}, mutation.attempts, mutation.frames, false); err == nil {
						t.Fatalf("session mutation escaped: %s", mutation.name)
					}
				})
			}
		})
	}
}
