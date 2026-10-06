package runtime

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestOpenAICompatibleArgumentsMatchProdexProfileProvider(t *testing.T) {
	arguments, err := openAICompatibleArguments("http://127.0.0.1:11434/v1", []string{"exec", "hello"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(arguments, "\n")
	for _, expected := range []string{
		`model_provider="godex-openai-compatible"`,
		`model_providers.godex-openai-compatible.name="OpenAI-compatible"`,
		`model_providers.godex-openai-compatible.base_url="http://127.0.0.1:11434/v1"`,
		`model_providers.godex-openai-compatible.wire_api="responses"`,
		`model_providers.godex-openai-compatible.requires_openai_auth=true`,
		`model_providers.godex-openai-compatible.supports_websockets=false`,
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("arguments missing %q: %#v", expected, arguments)
		}
	}
	if strings.Join(arguments[len(arguments)-2:], " ") != "exec hello" {
		t.Fatalf("user arguments moved: %#v", arguments)
	}
}

func TestOpenAICompatibleUserModelProviderOverrideWins(t *testing.T) {
	input := []string{"-c", `model_provider="custom"`, "exec", "hello"}
	arguments, err := openAICompatibleArguments("https://example.test/v1", input)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(arguments, "\n") != strings.Join(input, "\n") {
		t.Fatalf("user provider override was changed: %#v", arguments)
	}
}

func TestRunOpenAICompatibleProfileRunsDirectWithoutProxy(t *testing.T) {
	process := &fakeProcess{}
	runner := NewRunner(&fakeLaunchAccounts{}, process, nil)
	home := t.TempDir()
	if err := runner.RunOpenAICompatibleProfile(context.Background(), home, "https://example.test/v1", []string{"exec", "hello"}); err != nil {
		t.Fatal(err)
	}
	if len(process.homes) != 1 || process.homes[0] != home || len(process.args) != 1 || !strings.Contains(strings.Join(process.args[0], "\n"), `model_provider="godex-openai-compatible"`) {
		t.Fatalf("direct process = homes:%#v args:%#v", process.homes, process.args)
	}
}

func TestProdex04356RunOpenAICompatibleProfileWithOptionsUsesRuntimeProxy(t *testing.T) {
	home := t.TempDir()
	process := &fakeProxyProcess{}
	proxy := &fakeProxy{}
	var captured proxymodel.Config
	runner := NewRunner(nil, process, func(got proxymodel.Config) (Proxy, error) {
		captured = got
		return proxy, nil
	})
	runner.SetManagedProfilesRoot(filepath.Join(t.TempDir(), "profiles"))

	if err := runner.RunOpenAICompatibleProfileWithOptions(
		t.Context(), home, "https://example.test/v1",
		[]string{"exec", "hello"},
		RuntimeLaunchOptions{SuperOverlay: true, SmartContextEnabled: true},
	); err != nil {
		t.Fatal(err)
	}
	if captured.Provider.Kind != "openai-compatible" ||
		captured.Provider.APIURL != "https://example.test/v1" ||
		!captured.SmartContextEnabled {
		t.Fatalf("compatible runtime config = %#v", captured)
	}
	if process.home == home ||
		process.endpoint != "http://127.0.0.1:1234" ||
		!strings.Contains(strings.Join(process.arguments, "\n"), "exec") {
		t.Fatalf("compatible child = home:%q endpoint:%q args:%#v", process.home, process.endpoint, process.arguments)
	}
}
