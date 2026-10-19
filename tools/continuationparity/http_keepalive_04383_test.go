package main

import (
	"testing"
	"time"
)

func TestProdex04383HTTPKeepAliveOracleRejectsMutations(t *testing.T) {
	base := httpKeepAliveResult04383{
		replies:     []exchange{{200, `{"id":"resp-main","object":"response","output":[]}`}, {200, `{"id":"resp-main","object":"response","output":[]}`}},
		attempts:    []attempt{{account: "account-a", body: `{"input":[]}`}, {account: "account-a", body: `{"input":[]}`}},
		connections: []int{1, 1}, reused: true, sameConnection: true, elapsed: 31 * time.Second,
	}
	if err := verifyHTTPKeepAlive04383(base); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*httpKeepAliveResult04383){
		"old budget":                func(r *httpKeepAliveResult04383) { r.replies[1].status = 504 },
		"new socket":                func(r *httpKeepAliveResult04383) { r.sameConnection = false },
		"no reuse":                  func(r *httpKeepAliveResult04383) { r.reused = false },
		"automatic transport retry": func(r *httpKeepAliveResult04383) { r.connections[1]++ },
		"missing inference":         func(r *httpKeepAliveResult04383) { r.attempts = r.attempts[:1] },
		"extra inference":           func(r *httpKeepAliveResult04383) { r.attempts = append(r.attempts, r.attempts[1]) },
		"wrong owner":               func(r *httpKeepAliveResult04383) { r.attempts[1].account = "account-b" },
		"changed input":             func(r *httpKeepAliveResult04383) { r.attempts[1].body = `{}` },
		"changed response":          func(r *httpKeepAliveResult04383) { r.replies[1].body = `{}` },
		"unexpired original budget": func(r *httpKeepAliveResult04383) { r.elapsed = 29 * time.Second },
	} {
		t.Run(name, func(t *testing.T) {
			got := base
			got.replies = append([]exchange(nil), base.replies...)
			got.attempts = append([]attempt(nil), base.attempts...)
			got.connections = append([]int(nil), base.connections...)
			mutate(&got)
			if err := verifyHTTPKeepAlive04383(got); err == nil {
				t.Fatal("HTTP keepalive mutation escaped the oracle")
			}
		})
	}
}
