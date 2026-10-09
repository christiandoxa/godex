package runtime

import (
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04371ProviderRoutingIdentityChangesWithEndpoint(t *testing.T) {
	home := "/synthetic/profile"
	first := providerRoutingID(home, proxymodel.Provider{
		Kind:   "openai-compatible",
		APIURL: "https://upstream-one.example/v1",
	})
	second := providerRoutingID(home, proxymodel.Provider{
		Kind:   "openai-compatible",
		APIURL: "https://upstream-two.example/v1",
	})
	if first == second {
		t.Fatalf("provider endpoint migration reused routing identity %q", first)
	}
	if first != providerRoutingID(home, proxymodel.Provider{
		Kind:   "openai-compatible",
		APIURL: "https://upstream-one.example/v1",
	}) {
		t.Fatal("unchanged provider endpoint did not retain a stable routing identity")
	}
}
