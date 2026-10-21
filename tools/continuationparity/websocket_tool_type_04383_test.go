package main

import (
	"strings"
	"testing"
)

func TestProdex04383WebSocketToolSuffixOracleRejectsMutations(t *testing.T) {
	for _, shape := range []string{"canonical", "trailing-space", "trailing-tab", "leading-space", "serde-max-invalid", "serde-long-valid"} {
		t.Run(shape, func(t *testing.T) {
			body := websocketToolTypeMessage04383(shape)
			attempt := transportAttempt04383{owner: "account-a", ready: true, body: body}
			var attempts []transportAttempt04383
			var handshakes []string
			for index := range websocketToolTypeAttempts04383(shape) {
				item := attempt
				if shape == "serde-max-invalid" && index%2 == 1 {
					item.owner = "account-b"
				}
				attempts = append(attempts, item)
				handshakes = append(handshakes, item.owner)
			}
			frames := []string{`{"type":"error","status":409,"error":{"code":"stale_continuation"}}`}
			if err := verifyWebSocketToolType04383(shape, body, handshakes, attempts, frames, false); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				name   string
				mutate func(*[]string, *[]transportAttempt04383, *[]string, *bool)
			}{
				{"wrong owner", func(h *[]string, a *[]transportAttempt04383, f *[]string, b *bool) { (*a)[0].owner = "account-b" }},
				{"unready", func(h *[]string, a *[]transportAttempt04383, f *[]string, b *bool) { (*a)[0].ready = false }},
				{"extra turn", func(h *[]string, a *[]transportAttempt04383, f *[]string, b *bool) { (*a)[0].turn = "forged" }},
				{"rewrote type", func(h *[]string, a *[]transportAttempt04383, f *[]string, b *bool) {
					(*a)[0].body = strings.Replace((*a)[0].body, "call-tool-suffix", "other", 1)
				}},
				{"wrong handshake", func(h *[]string, a *[]transportAttempt04383, f *[]string, b *bool) { (*h)[0] = "account-b" }},
				{"lost attempt", func(h *[]string, a *[]transportAttempt04383, f *[]string, b *bool) { *a = (*a)[:len(*a)-1] }},
				{"duplicated attempt", func(h *[]string, a *[]transportAttempt04383, f *[]string, b *bool) { *a = append(*a, (*a)[0]) }},
				{"lost terminal", func(h *[]string, a *[]transportAttempt04383, f *[]string, b *bool) { *f = nil }},
				{"forged successful terminal", func(h *[]string, a *[]transportAttempt04383, f *[]string, b *bool) {
					*f = []string{transportCompleted04383}
				}},
				{"invalid auth", func(h *[]string, a *[]transportAttempt04383, f *[]string, b *bool) { *b = true }},
			} {
				t.Run(tc.name, func(t *testing.T) {
					h := append([]string(nil), handshakes...)
					a := append([]transportAttempt04383(nil), attempts...)
					f := append([]string(nil), frames...)
					invalid := false
					tc.mutate(&h, &a, &f, &invalid)
					if err := verifyWebSocketToolType04383(shape, body, h, a, f, invalid); err == nil {
						t.Fatalf("WebSocket tool suffix oracle missed %s", tc.name)
					}
				})
			}
		})
	}
}
