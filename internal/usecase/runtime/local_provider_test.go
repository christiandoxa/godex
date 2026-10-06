package runtime

import (
	"context"
	"strings"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestLocalProviderArgumentsMatchProdexConfigContract(t *testing.T) {
	contextWindow := uint64(8192)
	autoCompact := uint64(7000)
	arguments, err := localProviderArguments(LocalProviderConfig{
		URL:                   "http://127.0.0.1:8131",
		Model:                 "qwen3-coder",
		ContextWindow:         &contextWindow,
		AutoCompactTokenLimit: &autoCompact,
	}, []string{"exec", "review"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`model_provider="godex-local"`,
		`model="qwen3-coder"`,
		`model_providers.godex-local.name="Godex Local"`,
		`model_providers.godex-local.base_url="http://127.0.0.1:8131/v1"`,
		`model_providers.godex-local.wire_api="responses"`,
		`model_providers.godex-local.requires_openai_auth=true`,
		`model_providers.godex-local.supports_websockets=false`,
		`model_context_window=8192`,
		`model_auto_compact_token_limit=7000`,
		`model_reasoning_summary="none"`,
		`web_search="disabled"`,
		`features.apps=false`,
		`features.js_repl=false`,
		`features.image_generation=false`,
	}
	if len(arguments) != len(want)*2+2 {
		t.Fatalf("arguments len = %d, want %d: %#v", len(arguments), len(want)*2+2, arguments)
	}
	for index, entry := range want {
		position := index * 2
		if arguments[position] != "-c" || arguments[position+1] != entry {
			t.Fatalf("config %d = %#v, want %q", index, arguments[position:position+2], entry)
		}
	}
	if tail := arguments[len(arguments)-2:]; tail[0] != "exec" || tail[1] != "review" {
		t.Fatalf("tail = %#v", tail)
	}
}

func TestLocalProviderArgumentsUseDefaultsAndClampCompactLimit(t *testing.T) {
	contextWindow := uint64(1000)
	autoCompact := uint64(2000)
	arguments, err := localProviderArguments(LocalProviderConfig{
		URL:                   "https://local.example.test/custom/",
		ContextWindow:         &contextWindow,
		AutoCompactTokenLimit: &autoCompact,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(arguments, "\n")
	for _, expected := range []string{
		`model="unsloth/qwen3.5-35b-a3b"`,
		`model_providers.godex-local.base_url="https://local.example.test/custom"`,
		`model_context_window=1000`,
		`model_auto_compact_token_limit=999`,
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("arguments missing %q: %#v", expected, arguments)
		}
	}

	one, zero := uint64(1), uint64(0)
	defaults, err := localProviderArguments(LocalProviderConfig{
		URL:                   "https://local.example.test",
		ContextWindow:         &one,
		AutoCompactTokenLimit: &zero,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	joined = strings.Join(defaults, "\n")
	if !strings.Contains(joined, "model_context_window=16384") ||
		!strings.Contains(joined, "model_auto_compact_token_limit=14000") {
		t.Fatalf("default limits = %#v", defaults)
	}
}

func TestRunLocalProviderBypassesProxyAndQuotaSelection(t *testing.T) {
	home := t.TempDir()
	accounts := activeLaunchAccounts{&fakeLaunchAccounts{
		accounts:  []accountentity.Account{{ID: "active", Name: "active", Enabled: true}},
		homes:     map[string]string{"active": home},
		selectErr: nil,
	}}
	process := &fakeProcess{}
	proxyCalled := false
	runner := NewRunner(accounts, process, func(proxymodel.Config) (Proxy, error) {
		proxyCalled = true
		return &fakeProxy{}, nil
	})
	runner.SetQuotaPreflight(&fakeQuotaPreflight{ready: map[string]bool{}})
	if err := runner.RunLocalProvider(context.Background(), "", LocalProviderConfig{
		URL: "http://127.0.0.1:8131",
	}, []string{"exec", "hello"}); err != nil {
		t.Fatal(err)
	}
	if proxyCalled || len(process.homes) != 1 || process.homes[0] != home {
		t.Fatalf("local provider used proxy/home = %t / %#v", proxyCalled, process.homes)
	}
	if len(process.args) != 1 || !strings.Contains(strings.Join(process.args[0], "\n"), `model_provider="godex-local"`) {
		t.Fatalf("process args = %#v", process.args)
	}
}

func TestApplyProviderSelectionLimitsUsesOverridesAndClamps(t *testing.T) {
	provider := AnthropicProvider("fixture", "")
	contextWindow, autoCompact := uint64(4096), uint64(5000)
	if err := ApplyProviderSelectionLimits(&provider, &contextWindow, &autoCompact); err != nil {
		t.Fatal(err)
	}
	if provider.ContextWindow != 4096 || provider.AutoCompactLimit != 4095 {
		t.Fatalf("provider limits = %#v", provider)
	}
}

func TestRunLocalProviderFallsBackToCurrentCodexHomeWithoutManagedAccount(t *testing.T) {
	home := t.TempDir()
	process := &fakeProcess{}
	runner := NewRunner(&fakeLaunchAccounts{}, process, nil)
	runner.SetCurrentCodexHome(home)
	if err := runner.RunLocalProvider(context.Background(), "", LocalProviderConfig{
		URL: "http://127.0.0.1:8131",
	}, []string{"exec", "hello"}); err != nil {
		t.Fatal(err)
	}
	if len(process.homes) != 1 || process.homes[0] != home {
		t.Fatalf("current-home fallback = %#v", process.homes)
	}
	if err := runner.RunLocalProvider(context.Background(), "missing", LocalProviderConfig{
		URL: "http://127.0.0.1:8131",
	}, nil); err == nil {
		t.Fatal("explicit missing selector unexpectedly fell back to current home")
	}
}
