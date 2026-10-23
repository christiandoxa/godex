package main

import (
	"slices"
	"testing"
)

func TestSuperResumeOracleRejectsLaunchMutations(t *testing.T) {
	for _, local := range []bool{false, true} {
		valid := superResumeChild04390{
			Arguments: []string{"exec", "resume", "--dangerously-bypass-approvals-and-sandbox", legacySession04390, "continue"},
			Home:      "/synthetic/overlay", AuthKind: "profile", SessionVisible: true,
		}
		if local {
			valid.AuthKind = "synthetic"
		}
		if err := verifySuperResumeChild04390(valid, "/synthetic/profile", local); err != nil {
			t.Fatal(err)
		}
		for _, mutation := range []string{"command", "target", "prompt", "access", "duplicate", "home", "auth", "rollout", "saved_model", "saved_effort"} {
			changed := valid
			changed.Arguments = slices.Clone(valid.Arguments)
			switch mutation {
			case "command":
				changed.Arguments[1] = "fork"
			case "target":
				changed.Arguments[3] = nativeSession04390
			case "prompt":
				changed.Arguments[4] = "replay"
			case "access":
				changed.Arguments[2] = "--full-auto"
			case "duplicate":
				changed.Arguments = append([]string{legacySession04390}, changed.Arguments...)
			case "home":
				changed.Home = "/synthetic/profile"
			case "auth":
				changed.AuthKind = "unexpected"
			case "rollout":
				changed.SessionVisible = false
			case "saved_model":
				changed.Arguments = append([]string{"-c", `model="synthetic-saved-model"`}, changed.Arguments...)
			case "saved_effort":
				changed.Arguments = append([]string{"-c", `model_reasoning_effort="high"`}, changed.Arguments...)
			}
			if err := verifySuperResumeChild04390(changed, "/synthetic/profile", local); err == nil {
				t.Fatalf("accepted changed Super %s, local=%t", mutation, local)
			}
		}
	}
}
