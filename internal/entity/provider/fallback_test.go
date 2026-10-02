package provider

import (
	"reflect"
	"strings"
	"testing"
)

func TestModelFallbackChainMatchesProdexAnthropicAndCopilot(t *testing.T) {
	cases := []struct {
		provider, model string
		want            []string
	}{
		{"anthropic", "", []string{"claude-sonnet-5-5", "claude-opus-5-5", "claude-haiku-4-5"}},
		{"anthropic", "DEFAULT", []string{"claude-sonnet-5-5", "claude-opus-5-5", "claude-haiku-4-5"}},
		{"anthropic", "best", []string{"claude-opus-5-5", "claude-sonnet-5-5"}},
		{"anthropic", "pro", []string{"claude-sonnet-5-5", "claude-opus-5-5"}},
		{"anthropic", "flash", []string{"claude-haiku-4-5", "claude-sonnet-5-5"}},
		{"copilot", "", []string{"gpt-6-astra", "gpt-6.1-sol", "gpt-5.3-codex"}},
		{"copilot", "pro", []string{"gpt-6-astra", "gpt-6.1-sol", "gpt-5.3-codex"}},
		{"copilot", "astra", []string{"gpt-6-astra", "gpt-6.1-sol", "gpt-6-luna"}},
		{"copilot", "sol", []string{"gpt-6.1-sol", "gpt-6-sol", "gpt-6-luna"}},
		{"copilot", "luna", []string{"gpt-6-luna", "gpt-6.1-sol"}},
		{"copilot", "gpt-5.3-codex", []string{"gpt-5.3-codex", "gpt-6.1-sol", "gpt-6-luna"}},
		{"copilot", "sonnet", []string{"claude-sonnet-5-5", "gpt-6.1-sol", "gpt-5.3-codex"}},
		{"copilot", "gemini", []string{"gemini-3.8-flash", "gpt-6.1-sol", "gpt-5.3-codex"}},
	}
	for _, fixture := range cases {
		if got := ModelFallbackChain(fixture.provider, fixture.model); !reflect.DeepEqual(got, fixture.want) {
			t.Fatalf("provider=%s model=%q got=%#v want=%#v", fixture.provider, fixture.model, got, fixture.want)
		}
	}
}

func TestModelFallbackChainPreservesProdexComboSemantics(t *testing.T) {
	cases := []struct {
		model string
		want  []string
	}{
		{"combo:Alpha, alpha;Beta|gamma>beta", []string{"Alpha", "Beta", "gamma"}},
		{"combo:,,,", []string{"combo:,,,"}},
		{"combo: \u3000Alpha\t, Beta", []string{"Alpha", "Beta"}},
		{"COMBO:Alpha,beta", []string{"COMBO:Alpha,beta"}},
		{" \u2003模型-custom\u3000 ", []string{"模型-custom"}},
	}
	for _, fixture := range cases {
		if got := ModelFallbackChain("openai", fixture.model); !reflect.DeepEqual(got, fixture.want) {
			t.Fatalf("model=%q got=%#v want=%#v", fixture.model, got, fixture.want)
		}
	}
}

func TestDeepSeekFallbackChainMatchesProdex(t *testing.T) {
	fixtures := map[string][]string{
		"":              {"deepseek-v4-pro", "deepseek-v4-flash"},
		"auto":          {"deepseek-v4-pro", "deepseek-v4-flash"},
		"pro":           {"deepseek-v4-pro", "deepseek-v4-flash"},
		"flash":         {"deepseek-v4-flash", "deepseek-v4-pro"},
		"deepseek-chat": {"deepseek-chat"},
	}
	for model, want := range fixtures {
		got := ModelFallbackChain("deepseek", model)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("model %q chain = %v, want %v", model, got, want)
		}
	}
}
