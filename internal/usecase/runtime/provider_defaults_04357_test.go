package runtime

import (
	"reflect"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04357ProviderRuntimeFeatureDefaultsMatchCanonicalMetadata(t *testing.T) {
	cases := []struct {
		name            string
		provider        proxymodel.Provider
		webSearch       string
		imageGeneration string
	}{
		{name: "local", provider: proxymodel.Provider{Kind: "local"}, webSearch: "disabled", imageGeneration: "false"},
		{name: "deepseek", provider: DeepSeekProvider("deepseek", ""), webSearch: "live", imageGeneration: "false"},
		{name: "gemini", provider: GeminiProvider("gemini", ""), webSearch: "live", imageGeneration: "true"},
		{name: "anthropic", provider: AnthropicProvider("anthropic", ""), webSearch: "live", imageGeneration: "false"},
		{name: "copilot", provider: CopilotProvider("copilot", "", "", ""), webSearch: "live", imageGeneration: "false"},
		{name: "kiro", provider: KiroProvider("kiro"), webSearch: "live", imageGeneration: "false"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := prepareProviderRuntimeArguments(nil, "", tc.provider, []string{"exec", "hello"})
			if err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(got, "\n")
			for _, want := range []string{
				`model_reasoning_summary="none"`,
				`web_search="` + tc.webSearch + `"`,
				"features.apps=false",
				"features.js_repl=false",
				"features.image_generation=" + tc.imageGeneration,
			} {
				if !strings.Contains(joined, want) {
					t.Fatalf("%s runtime args missing %q: %#v", tc.name, want, got)
				}
			}
		})
	}
}

func TestProdex04357ProviderRuntimeFeatureDefaultsLeaveOpenAIUntouched(t *testing.T) {
	input := []string{"exec", "hello"}
	got, err := prepareProviderRuntimeArguments(nil, "", proxymodel.Provider{}, input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, input) {
		t.Fatalf("OpenAI runtime args = %#v, want %#v", got, input)
	}
}
