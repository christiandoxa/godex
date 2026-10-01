package quota

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type fakeRedeemRunner struct {
	input    quotamodel.RedeemInput
	plan     quotamodel.RedeemPlan
	result   quotamodel.RedeemResult
	executed bool
}

func (fake *fakeRedeemRunner) Prepare(_ context.Context, input quotamodel.RedeemInput) (quotamodel.RedeemPlan, error) {
	fake.input = input
	return fake.plan, nil
}

func (fake *fakeRedeemRunner) Execute(_ context.Context, plan quotamodel.RedeemPlan) (quotamodel.RedeemResult, error) {
	fake.executed = true
	if plan.Profile != fake.plan.Profile {
		return quotamodel.RedeemResult{}, errors.New("unexpected plan")
	}
	return fake.result, nil
}

func TestRedeemParsesFlagsAndPrintsResult(t *testing.T) {
	runner := &fakeRedeemRunner{
		plan:   quotamodel.RedeemPlan{Profile: "main"},
		result: quotamodel.RedeemResult{Profile: "main", Outcome: quotamodel.RedeemReset, RequestID: "prodex-manual-redeem-fixture"},
	}
	var output strings.Builder
	err := redeemWithIO(context.Background(), runner, &output, strings.NewReader(""), &strings.Builder{}, false,
		[]string{"main", "--yes", "--base-url", "https://quota.test/backend-api", "--no-proxy"})
	if err != nil {
		t.Fatal(err)
	}
	if runner.input != (quotamodel.RedeemInput{Profile: "main", BaseURL: "https://quota.test/backend-api", NoProxy: true}) || !runner.executed {
		t.Fatalf("input/executed = %+v / %t", runner.input, runner.executed)
	}
	if output.String() != "profile=main outcome=reset request_id=prodex-manual-redeem-fixture\n" {
		t.Fatalf("output = %q", output.String())
	}
}

func TestRedeemNearResetRequiresYesWhenNonInteractive(t *testing.T) {
	near := quotamodel.NearReset{Label: "weekly", ResetAt: 1_700_000_000}
	runner := &fakeRedeemRunner{plan: quotamodel.RedeemPlan{Profile: "main", NearReset: &near}}
	err := redeemWithIO(context.Background(), runner, &strings.Builder{}, strings.NewReader(""), &strings.Builder{}, false, []string{"main"})
	if err == nil || !strings.Contains(err.Error(), "pass --yes") || runner.executed {
		t.Fatalf("error/executed = %v / %t", err, runner.executed)
	}
}

func TestRedeemInteractiveConfirmation(t *testing.T) {
	near := quotamodel.NearReset{Label: "5h", ResetAt: 1_700_000_000}
	runner := &fakeRedeemRunner{
		plan:   quotamodel.RedeemPlan{Profile: "main", NearReset: &near},
		result: quotamodel.RedeemResult{Profile: "main", Outcome: quotamodel.RedeemNoCredit, RequestID: "prodex-manual-redeem-fixture"},
	}
	var prompt strings.Builder
	if err := redeemWithIO(context.Background(), runner, &strings.Builder{}, strings.NewReader("yes\n"), &prompt, true, []string{"main"}); err != nil {
		t.Fatal(err)
	}
	if !runner.executed || !strings.Contains(prompt.String(), "Redeem one reset credit") {
		t.Fatalf("executed/prompt = %t / %q", runner.executed, prompt.String())
	}
}

func TestParseRedeemConfirmation(t *testing.T) {
	for input, want := range map[string]bool{"": false, "no": false, "yes": true, "Y": true} {
		got, valid := parseRedeemConfirmation(input)
		if !valid || got != want {
			t.Fatalf("confirmation %q = %t/%t", input, got, valid)
		}
	}
	if _, valid := parseRedeemConfirmation("wat"); valid {
		t.Fatal("invalid confirmation accepted")
	}
}

func TestRedeemPromptTUIUsesReferenceConfirmationKeys(t *testing.T) {
	base := redeemPromptModel{profile: "work", label: "5-hour", resetTime: "2026-10-01T12:00:00Z"}
	for _, test := range []struct {
		key       tea.KeyMsg
		confirmed bool
	}{
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}, true},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Y'}}, true},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}, false},
		{tea.KeyMsg{Type: tea.KeyEnter}, false},
		{tea.KeyMsg{Type: tea.KeyEsc}, false},
	} {
		updated, command := base.Update(test.key)
		model := updated.(redeemPromptModel)
		if command == nil || model.confirmed != test.confirmed {
			t.Fatalf("key %q = confirmed %t, command=%v", test.key.String(), model.confirmed, command)
		}
	}
	view := base.View()
	for _, expected := range []string{"Godex Redeem", "Quota reset is near", "work", "y redeem", "enter/esc cancel"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("redeem TUI missing %q: %q", expected, view)
		}
	}
}
