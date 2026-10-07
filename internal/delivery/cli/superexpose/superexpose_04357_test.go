package superexpose

import (
	"strings"
	"testing"
)

func TestProdex04357OpenAITunnelQualificationReferenceIs0016(t *testing.T) {
	if openAITunnelMinimumVersion != "0.0.13" {
		t.Fatalf("minimum tunnel-client version = %q", openAITunnelMinimumVersion)
	}
	if openAITunnelLatestReference != "0.0.16" {
		t.Fatalf("latest qualified tunnel-client reference = %q, want 0.0.16", openAITunnelLatestReference)
	}
	if text := openAITunnelInstallError().Error(); !strings.Contains(text, "0.0.16") || !strings.Contains(text, "0.0.13") {
		t.Fatalf("tunnel install guidance = %q", text)
	}
	const sha = "5f99daabd4aa4a77049e6d81d54a0d8c18335397"
	version, ok := supportedTunnelClientVersion("0.0.16+" + sha + " (git sha: " + sha + ")")
	if !ok || version != "0.0.16" {
		t.Fatalf("official 0.0.16 qualification = %q / %t", version, ok)
	}
}
