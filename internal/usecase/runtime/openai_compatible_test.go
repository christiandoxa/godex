package runtime

import (
	"context"
	"strings"
	"testing"
)

func TestOpenAICompatibleArgumentsMatchProdexProfileProvider(t *testing.T) {
	arguments, err := openAICompatibleArguments("http://127.0.0.1:11434/v1", []string{"exec", "hello"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(arguments, "\n")
	for _, expected := range []string{
		`model_provider="prodex-openai-compatible"`,
		`model_providers.prodex-openai-compatible.name="OpenAI-compatible"`,
		`model_providers.prodex-openai-compatible.base_url="http://127.0.0.1:11434/v1"`,
		`model_providers.prodex-openai-compatible.wire_api="responses"`,
		`model_providers.prodex-openai-compatible.requires_openai_auth=true`,
		`model_providers.prodex-openai-compatible.supports_websockets=false`,
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
	if len(process.homes) != 1 || process.homes[0] != home || len(process.args) != 1 || !strings.Contains(strings.Join(process.args[0], "\n"), `model_provider="prodex-openai-compatible"`) {
		t.Fatalf("direct process = homes:%#v args:%#v", process.homes, process.args)
	}
}
