package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// Prodex 0.437.14's UsageFetchFlow sends at most three credential attempts:
// original, a changed on-disk credential after 401, then a refreshed token
// after another 401. Its receipt identifies the *final* accepted credential,
// not the original failed token or a later unrelated on-disk read.
func TestProdex043714QuotaUnauthorizedReloadUsesAcceptedIdentity(t *testing.T) {
	first := proxymodel.Auth{AccessToken: "synthetic-a", AccountID: "owner"}
	second := proxymodel.Auth{AccessToken: "synthetic-b", AccountID: "owner"}
	var mu sync.Mutex
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		mu.Lock()
		requests = append(requests, token)
		mu.Unlock()
		if token != "Bearer synthetic-b" {
			w.WriteHeader(401)
			return
		}
		_, _ = w.Write([]byte(`{"rate_limit":{"primary_window":{"used_percent":21}}}`))
	}))
	defer server.Close()
	reader := &authReloadSequence{values: []proxymodel.Auth{first, second, second}}
	client, err := NewQuotaClient(server.URL+"/backend-api", nil, reader)
	if err != nil {
		t.Fatal(err)
	}
	usage, receipt, err := client.FetchAuthenticatedAtPolicy(t.Context(), "/synthetic", "", false)
	expected, _ := openAIAuthDigest043714(second)
	old, _ := openAIAuthDigest043714(first)
	if err != nil || !receipt.Confirmed || receipt.CredentialDigest != expected || receipt.CredentialDigest == old ||
		usage.Primary == nil || usage.Primary.UsedPercent == nil || *usage.Primary.UsedPercent != 21 {
		t.Fatalf("successful reload used stale credential: receipt=%+v usage=%+v err=%v", receipt, usage, err)
	}
	mu.Lock()
	got := append([]string(nil), requests...)
	mu.Unlock()
	if want := []string{"Bearer synthetic-a", "Bearer synthetic-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("reload did not use one extra request: got=%v, want=%v", got, want)
	}
}

func TestProdex043714QuotaUnauthorizedRefreshUsesAcceptedIdentity(t *testing.T) {
	first := proxymodel.Auth{AccessToken: "synthetic-a", AccountID: "owner"}
	second := proxymodel.Auth{AccessToken: "synthetic-b", AccountID: "owner"}
	third := proxymodel.Auth{AccessToken: "synthetic-c", AccountID: "owner"}
	var mu sync.Mutex
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		mu.Lock()
		requests = append(requests, token)
		mu.Unlock()
		if token != "Bearer synthetic-c" {
			w.WriteHeader(401)
			return
		}
		_, _ = w.Write([]byte(`{"rate_limit":{"primary_window":{"used_percent":29}}}`))
	}))
	defer server.Close()
	reader := &authRefreshSequence{authReloadSequence: authReloadSequence{values: []proxymodel.Auth{first, second, third}}, refreshed: third}
	client, err := NewQuotaClient(server.URL+"/backend-api", nil, reader)
	if err != nil {
		t.Fatal(err)
	}
	usage, receipt, err := client.FetchAuthenticatedAtPolicy(t.Context(), "/synthetic", "", false)
	expected, _ := openAIAuthDigest043714(third)
	if err != nil || !receipt.Confirmed || receipt.CredentialDigest != expected ||
		usage.Primary == nil || usage.Primary.UsedPercent == nil || *usage.Primary.UsedPercent != 29 {
		t.Fatalf("successful refresh used stale credential: receipt=%+v usage=%+v err=%v", receipt, usage, err)
	}
	mu.Lock()
	got := append([]string(nil), requests...)
	mu.Unlock()
	if want := []string{"Bearer synthetic-a", "Bearer synthetic-b", "Bearer synthetic-c"}; !reflect.DeepEqual(got, want) || reader.refreshCalls != 1 {
		t.Fatalf("unauthorized refresh failed request count/identity: got=%v calls=%d", got, reader.refreshCalls)
	}
}

func TestProdex043714QuotaUnauthorizedNeverAcceptsStaleCredential(t *testing.T) {
	reader := &authReloadSequence{values: []proxymodel.Auth{{AccessToken: "synthetic-a", AccountID: "owner"}}}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls++; w.WriteHeader(401) }))
	defer server.Close()
	client, err := NewQuotaClient(server.URL+"/backend-api", nil, reader)
	if err != nil {
		t.Fatal(err)
	}
	_, receipt, err := client.FetchAuthenticatedAtPolicy(context.Background(), "/synthetic", "", false)
	if err == nil || receipt.Confirmed || calls != 1 {
		t.Fatalf("terminal 401 forged receipt/replayed same credential: calls=%d receipt=%+v err=%v", calls, receipt, err)
	}
}
