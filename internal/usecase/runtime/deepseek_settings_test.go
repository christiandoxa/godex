package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestDeepSeekRuntimeSettingsPreferConfigOverEnvironment(t *testing.T) {
	home := t.TempDir()
	content := `[deepseek]
strict_tools = true
web_search_mode = "off"
beta_base_url = "https://deepseek.example.test/beta/"
`
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	lookup := func(key string) (string, bool) {
		values := map[string]string{
			"GODEX_DEEPSEEK_STRICT_TOOLS":    "false",
			"GODEX_DEEPSEEK_WEB_SEARCH_MODE": "auto",
			"GODEX_DEEPSEEK_BETA_BASE_URL":   "https://env.example.test/beta",
		}
		value, ok := values[key]
		return value, ok
	}
	settings, err := deepSeekRuntimeSettings(home, lookup)
	if err != nil {
		t.Fatal(err)
	}
	if !settings.strictTools || settings.webSearchMode != "off" || settings.betaBaseURL != "https://deepseek.example.test/beta" {
		t.Fatalf("settings = %#v", settings)
	}
}

func TestDeepSeekRuntimeSettingsEnvironmentAndDefaultsMatchProdex(t *testing.T) {
	empty := func(string) (string, bool) { return "", false }
	settings, err := deepSeekRuntimeSettings(t.TempDir(), empty)
	if err != nil {
		t.Fatal(err)
	}
	if settings.strictTools || settings.webSearchMode != "auto" || settings.betaBaseURL != "https://api.deepseek.com/beta" || settings.sseLookaheadTimeout != time.Second {
		t.Fatalf("default settings = %#v", settings)
	}

	values := map[string]string{
		"GODEX_DEEPSEEK_STRICT_TOOLS":                  "YES",
		"GODEX_DEEPSEEK_WEB_SEARCH_MODE":               "openai_chat",
		"GODEX_DEEPSEEK_BETA_BASE_URL":                 "https://env.example.test/beta/",
		"GODEX_RUNTIME_PROXY_SSE_LOOKAHEAD_TIMEOUT_MS": "1250",
	}
	settings, err = deepSeekRuntimeSettings(t.TempDir(), func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
	if err != nil {
		t.Fatal(err)
	}
	if !settings.strictTools || settings.webSearchMode != "openai_chat" || settings.betaBaseURL != "https://env.example.test/beta" || settings.sseLookaheadTimeout != 1250*time.Millisecond {
		t.Fatalf("environment settings = %#v", settings)
	}
}

func TestDeepSeekRuntimeSettingsAcceptsProdexWebSearchModes(t *testing.T) {
	for _, mode := range []string{"auto", "off", "openai_chat", "anthropic"} {
		settings, err := deepSeekRuntimeSettings(t.TempDir(), func(key string) (string, bool) {
			return mode, key == "GODEX_DEEPSEEK_WEB_SEARCH_MODE"
		})
		if err != nil || settings.webSearchMode != mode {
			t.Fatalf("mode %q settings = %#v, error = %v", mode, settings, err)
		}
	}
}

func TestPrepareProviderLaunchCarriesDeepSeekConfigIntoProxyProvider(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(`[deepseek]
strict_tools = true
web_search_mode = "anthropic"
beta_base_url = "https://config.example.test/beta"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GODEX_DEEPSEEK_STRICT_TOOLS", "false")
	t.Setenv("GODEX_DEEPSEEK_WEB_SEARCH_MODE", "openai_chat")
	t.Setenv("GODEX_DEEPSEEK_BETA_BASE_URL", "https://env.example.test/beta")
	t.Setenv("GODEX_RUNTIME_PROXY_SSE_LOOKAHEAD_TIMEOUT_MS", "1250")

	provider, profiles, err := (&Runner{}).prepareProviderLaunch(home, proxymodel.Provider{Kind: "deepseek"}, []proxymodel.Account{{ID: "key"}})
	if err != nil {
		t.Fatal(err)
	}
	if !provider.StrictTools || provider.WebSearchMode != "anthropic" || provider.BetaBaseURL != "https://config.example.test/beta" || provider.SSELookaheadTimeout != 1250*time.Millisecond {
		t.Fatalf("DeepSeek runtime provider = %#v", provider)
	}
	if len(profiles) != 1 || !profiles[0].Provider.StrictTools || profiles[0].Provider.WebSearchMode != "anthropic" || profiles[0].Provider.BetaBaseURL != provider.BetaBaseURL || profiles[0].Provider.SSELookaheadTimeout != provider.SSELookaheadTimeout {
		t.Fatalf("DeepSeek proxy profiles = %#v", profiles)
	}
}

func TestDeepSeekRuntimeSettingsRejectInvalidValues(t *testing.T) {
	fixtures := []struct {
		key, value, want string
	}{
		{"GODEX_DEEPSEEK_STRICT_TOOLS", " true ", "must not contain whitespace"},
		{"GODEX_DEEPSEEK_STRICT_TOOLS", "maybe", "must be true or false"},
		{"GODEX_DEEPSEEK_WEB_SEARCH_MODE", "enabled", "must be auto, off, openai_chat, or anthropic"},
		{"GODEX_DEEPSEEK_BETA_BASE_URL", "https://user:pass@example.test", "must be an http(s) URL"},
		{"GODEX_RUNTIME_PROXY_SSE_LOOKAHEAD_TIMEOUT_MS", "0", "must be greater than zero"},
		{"GODEX_RUNTIME_PROXY_SSE_LOOKAHEAD_TIMEOUT_MS", "nope", "must be an unsigned integer"},
	}
	for _, fixture := range fixtures {
		_, err := deepSeekRuntimeSettings(t.TempDir(), func(key string) (string, bool) {
			if key == fixture.key {
				return fixture.value, true
			}
			return "", false
		})
		if err == nil || !strings.Contains(err.Error(), fixture.want) {
			t.Fatalf("%s=%q error = %v", fixture.key, fixture.value, err)
		}
	}
}

func TestDeepSeekStrictToolsRejectsNonBooleanConfig(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("[deepseek]\nstrict_tools = []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := deepSeekRuntimeSettings(home, func(string) (string, bool) { return "true", true })
	if err == nil || !strings.Contains(err.Error(), "deepseek.strict_tools must be a boolean") {
		t.Fatalf("error = %v", err)
	}
}

func TestDeepSeekRuntimeSettingsFollowConfigSymlinkLikeProdex(t *testing.T) {
	target := filepath.Join(t.TempDir(), "shared-config.toml")
	if err := os.WriteFile(target, []byte(`[deepseek]
web_search_mode = "anthropic"
beta_base_url = "https://shared.example.test/beta"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	if err := os.Symlink(target, filepath.Join(home, "config.toml")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	settings, err := deepSeekRuntimeSettings(home, func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	if settings.webSearchMode != "anthropic" || settings.betaBaseURL != "https://shared.example.test/beta" {
		t.Fatalf("symlink settings = %#v", settings)
	}
}

func TestDeepSeekRuntimeSettingsAcceptsLegacyProdexEnvironmentAlias(t *testing.T) {
	values := map[string]string{
		"PRODEX_DEEPSEEK_STRICT_TOOLS":                  "true",
		"PRODEX_DEEPSEEK_WEB_SEARCH_MODE":               "anthropic",
		"PRODEX_DEEPSEEK_BETA_BASE_URL":                 "https://legacy.example.test/beta",
		"PRODEX_RUNTIME_PROXY_SSE_LOOKAHEAD_TIMEOUT_MS": "1500",
	}
	settings, err := deepSeekRuntimeSettings(t.TempDir(), func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
	if err != nil {
		t.Fatal(err)
	}
	if !settings.strictTools || settings.webSearchMode != "anthropic" ||
		settings.betaBaseURL != "https://legacy.example.test/beta" ||
		settings.sseLookaheadTimeout != 1500*time.Millisecond {
		t.Fatalf("legacy settings = %#v", settings)
	}
}
