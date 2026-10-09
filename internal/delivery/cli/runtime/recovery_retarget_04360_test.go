package runtime

import (
	"reflect"
	"testing"
)

const target04360 = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"

func TestProdex04360RetargetExecRecoveryMatchesTaggedMojo(t *testing.T) {
	cases := []struct {
		name          string
		initial, want []string
	}{
		{"fresh exec", []string{"--model", "gpt-5.6", "exec", "original prompt"},
			[]string{"--model", "gpt-5.6", "exec", "resume", target04360}},
		{"all exec options", []string{"exec", "--json", "--output-last-message", "/tmp/result", "-c", `model_reasoning_effort="max"`, "original prompt"},
			[]string{"exec", "resume", target04360, "--json", "--output-last-message", "/tmp/result", "-c", `model_reasoning_effort="max"`}},
		{"last with old prompt", []string{"exec", "resume", "--last", "old prompt"},
			[]string{"exec", "resume", target04360}},
		{"thread source removed", []string{"exec", "--thread-source", "automated_review", "-"},
			[]string{"exec", "resume", target04360}},
		{"cyber before resume", []string{"exec", "--cyber-access-program", "daybreak_blue", "-c", "daybreak=false", "original prompt"},
			[]string{"exec", "resume", target04360, "--cyber-access-program", "daybreak_blue", "-c", "daybreak=false"}},
		{"cyber after resume", []string{"exec", "resume", "00000000-0000-4000-8000-000000000001", "--cyber-access-program", "daybreak_red", "-c", "daybreak=false", "original prompt"},
			[]string{"exec", "resume", target04360, "--cyber-access-program", "daybreak_red", "-c", "daybreak=false"}},
		{"inline cyber and literal", []string{"exec", "--cyber-access-program=daybreak_blue", "resume", "00000000-0000-4000-8000-000000000001", "--", "literal --cyber-access-program text"},
			[]string{"exec", "resume", target04360, "--cyber-access-program=daybreak_blue"}},
		{"model ultra stays explicit", []string{"-m", "gpt-6.1-sol", "exec", "-c", `model_reasoning_effort="ultra"`, "do work"},
			[]string{"-m", "gpt-6.1-sol", "exec", "resume", target04360, "-c", `model_reasoning_effort="ultra"`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := retargetCodexExecRecovery04360(tc.initial, target04360)
			if !ok || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("tagged recovery args=%#v ok=%t want %#v", got, ok, tc.want)
			}
		})
	}
}

func TestProdex04360RetargetRecoveryDoesNotReplayUnapprovedCommands(t *testing.T) {
	for _, args := range [][]string{
		{"review", "--uncommitted"},
		{"exec", "review", "--uncommitted"},
		{"exec", "--", "resume", "id"},
		{"resume", "old-session"},
	} {
		if got, ok := retargetCodexExecRecovery04360(args, target04360); ok || got != nil {
			t.Fatalf("invalid headless session recovery accepted: %#v => %#v", args, got)
		}
	}
}

func TestProdex04361RetargetTUIRecoveryDropsPromptAndKeepsOptions(t *testing.T) {
	args := []string{
		"--model", "gpt-6.1-sol", "resume", target04360, "old prompt",
		"--no-alt-screen", "-C", "/synthetic/workspace",
	}
	got, ok := retargetCodexTUIRecovery04360(args, target04360)
	want := []string{
		"--model", "gpt-6.1-sol", "resume", target04360,
		"--no-alt-screen", "-C", "/synthetic/workspace",
	}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("TUI recovery args=%#v ok=%t want %#v", got, ok, want)
	}
	if _, ok := retargetCodexTUIRecovery04360([]string{"resume", "--last"}, target04360); ok {
		t.Fatal("unresolved TUI selector accepted")
	}
}
