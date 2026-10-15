package openai

import (
	"context"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex043714QuotaCurrentIdentityTracksRotatedCredential(t *testing.T) {
	first := proxymodel.Auth{AccessToken: "synthetic-first", AccountID: "synthetic-owner"}
	second := proxymodel.Auth{AccessToken: "synthetic-next", AccountID: "synthetic-owner"}
	reader := &authReloadSequence{values: []proxymodel.Auth{first, second}}
	client, err := NewQuotaClient("http://127.0.0.1:1/backend-api", nil, reader)
	if err != nil {
		t.Fatal(err)
	}
	a, err := client.CurrentCredentialDigest(context.Background(), "/synthetic")
	if err != nil {
		t.Fatal(err)
	}
	b, err := client.CurrentCredentialDigest(context.Background(), "/synthetic")
	if err != nil {
		t.Fatal(err)
	}
	expectedA, _ := openAIAuthDigest043714(first)
	expectedB, _ := openAIAuthDigest043714(second)
	if a != expectedA || b != expectedB || a == b || a == ([32]byte{}) || b == ([32]byte{}) {
		t.Fatal("active credential digest did not rotate with managed auth source")
	}
}
