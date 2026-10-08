package codex

import (
	"reflect"
	"slices"
	"testing"
)

func TestProdex04360CyberProgramPairPreservedByManagedProxyConfiguration(t *testing.T) {
	const session = "00000000-0000-4000-8000-000000000001"
	for _, program := range []string{"standard", "daybreak_blue", "daybreak_red"} {
		for _, tc := range []struct {
			name        string
			input, want []string
		}{
			{"root option", []string{"--cyber-access-program", program, "exec", "resume", session, "continue"}, []string{"--cyber-access-program", program, "exec", "resume", "managed", session, "continue"}},
			{"option after exec", []string{"exec", "--cyber-access-program", program, "resume", session, "continue"}, []string{"exec", "--cyber-access-program", program, "resume", "managed", session, "continue"}},
			{"option before session", []string{"exec", "resume", "--cyber-access-program", program, session, "continue"}, []string{"exec", "resume", "managed", "--cyber-access-program", program, session, "continue"}},
			{"option after session", []string{"exec", "resume", session, "--cyber-access-program", program, "continue"}, []string{"exec", "resume", "managed", session, "--cyber-access-program", program, "continue"}},
			{"inline value and literal", []string{"exec", "--cyber-access-program=" + program, "resume", session, "--", "literal --cyber-access-program text"}, []string{"exec", "--cyber-access-program=" + program, "resume", "managed", session, "--", "literal --cyber-access-program text"}},
		} {
			t.Run(program+"/"+tc.name, func(t *testing.T) {
				got := scopeModelArguments(tc.input, []string{"managed"})
				if !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("native Codex option/value pair split, input=%#v got=%#v want=%#v", tc.input, got, tc.want)
				}
			})
		}
	}
}

func TestProdex04360ManagedProxyDoesNotSplitCyberPairOrForceDaybreak(t *testing.T) {
	for _, pair := range [][]string{
		{"--cyber-access-program", "standard"},
		{"--cyber-access-program", "daybreak_blue"},
		{"--cyber-access-program", "daybreak_red"},
		{"--cyber-access-program=daybreak_blue"},
	} {
		args := append([]string{"exec"}, pair...)
		args = append(args, "resume", "00000000-0000-4000-8000-000000000001", "continue")
		got, err := proxyArguments("http://127.0.0.1:1234", args)
		if err != nil {
			t.Fatal(err)
		}
		start := slices.Index(got, pair[0])
		if start < 0 || len(got) < start+len(pair) || !reflect.DeepEqual(got[start:start+len(pair)], pair) {
			t.Fatalf("option/value pair split during governed proxy config, input=%#v result=%#v", args, got)
		}
		for _, element := range got {
			if element == "--enable" || element == "cli_daybreak" || element == "daybreak=true" {
				t.Fatalf("governed config forced Daybreak: %#v", got)
			}
		}
	}
}
