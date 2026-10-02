package auth

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestAPIKeyPromptModelMasksSecretAndMatchesReferenceStages(t *testing.T) {
	model := newAPIKeyPromptModel(LoginOptions{})
	for _, current := range "fixture-secret" {
		updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{current}})
		if command != nil {
			t.Fatal("typing secret unexpectedly returned command")
		}
		model = updated.(apiKeyPromptModel)
	}
	view := model.View()
	if strings.Contains(view, "fixture-secret") || !strings.Contains(view, strings.Repeat("*", len("fixture-secret"))) {
		t.Fatalf("secret prompt leaked or was not masked: %q", view)
	}
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(apiKeyPromptModel)
	if command != nil || model.stage != apiKeyPromptBaseURL || model.apiKey != "fixture-secret" {
		t.Fatalf("secret submit = %#v command=%v", model, command)
	}
	if !strings.Contains(model.View(), defaultOpenAIBaseURL) {
		t.Fatalf("base URL default missing: %q", model.View())
	}

	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(apiKeyPromptModel)
	if command != nil || model.stage != apiKeyPromptProfileName || model.baseURL != "" || !model.baseURLSpecified {
		t.Fatalf("base URL submit = %#v command=%v", model, command)
	}
	if !strings.Contains(model.View(), "api_key") {
		t.Fatalf("profile default missing: %q", model.View())
	}

	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(apiKeyPromptModel)
	if command == nil || model.stage != apiKeyPromptDone || model.profileName != "api_key" {
		t.Fatalf("profile submit = %#v command=%v", model, command)
	}
}

func TestAPIKeyPromptWithCLIBaseURLAndNameSkipsOptionalStages(t *testing.T) {
	model := newAPIKeyPromptModel(LoginOptions{
		Name: "work", BaseURL: "http://127.0.0.1:11434/v1", BaseURLSpecified: true,
	})
	model.input = "fixture-key"
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(apiKeyPromptModel)
	if command == nil || model.stage != apiKeyPromptDone || model.profileName != "work" || model.baseURL != "http://127.0.0.1:11434/v1" {
		t.Fatalf("fixed prompt = %#v command=%v", model, command)
	}
}

func TestAPIKeyPromptPlainFallbackReturnsProdexInput(t *testing.T) {
	input := strings.NewReader("fixture-key\nhttp://localhost:11434/v1\n\n")
	var output strings.Builder
	result, err := PromptAPIKeyLogin(context.Background(), input, &output, LoginOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.APIKey != "fixture-key" || result.BaseURL != "http://localhost:11434/v1" || !result.BaseURLSpecified || result.Name != "api_key_localhost" {
		t.Fatalf("result = %#v", result)
	}
	if strings.Contains(output.String(), "fixture-key") {
		t.Fatalf("prompt output leaked API key: %q", output.String())
	}
}

func TestAPIKeyPromptRejectsEmptyKeyAndCancellation(t *testing.T) {
	if _, err := PromptAPIKeyLogin(context.Background(), strings.NewReader("\n"), &strings.Builder{}, LoginOptions{}); err == nil || !strings.Contains(err.Error(), "cannot be empty") {
		t.Fatalf("empty key error = %v", err)
	}
	model := newAPIKeyPromptModel(LoginOptions{})
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(apiKeyPromptModel)
	if command == nil || !model.cancelled {
		t.Fatalf("cancel = %#v command=%v", model, command)
	}
}
