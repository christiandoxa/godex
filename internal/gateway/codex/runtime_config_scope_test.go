package codex

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestRuntimeChildrenKeepNativeConfigScopeOrder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("recording child uses a POSIX shell")
	}
	for _, direct := range []bool{false, true} {
		for _, fixture := range []struct {
			name  string
			input []string
			want  []string
		}{
			{"root exec", []string{"-c", `model_reasoning_effort="low"`, "exec", "--config", `model_reasoning_effort="high"`, "prompt"}, []string{"exec", "-c", `model_reasoning_effort="low"`, "--config", `model_reasoning_effort="high"`, "prompt"}},
			{"nested resume", []string{"--config=model_reasoning_effort=\"low\"", "exec", "-cmodel_reasoning_effort=\"high\"", "resume", "-c", `model_reasoning_effort="medium"`, "--last", "prompt"}, []string{"exec", "resume", "--config=model_reasoning_effort=\"low\"", "-cmodel_reasoning_effort=\"high\"", "-c", `model_reasoning_effort="medium"`, "--last", "prompt"}},
			{"literal tail", []string{"-c", "model_context_window=500000", "exec", "--", "--config", "literal=prompt"}, []string{"exec", "-c", "model_context_window=500000", "--", "--config", "literal=prompt"}},
			{"inline native", []string{"resume", "--last", "/model gpt-5.6 high"}, []string{"-c", "disable_paste_burst=true", "resume", "--last", "/model gpt-5.6 high"}},
		} {
			kind := "runtime/"
			if direct {
				kind = "direct/"
			}
			t.Run(kind+fixture.name, func(t *testing.T) {
				root := t.TempDir()
				if err := os.Mkdir(filepath.Join(root, "home"), 0o700); err != nil {
					t.Fatal(err)
				}
				record, binary := filepath.Join(root, "arguments"), filepath.Join(root, "codex")
				script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$GODEX_SCOPE_ARGUMENTS\"\n"
				if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
					t.Fatal(err)
				}
				t.Setenv("GODEX_SCOPE_ARGUMENTS", record)
				process := NewCodexProcess(binary, Terminal{Stdout: io.Discard, Stderr: io.Discard})
				original := append([]string(nil), fixture.input...)
				var err error
				if direct {
					err = process.RunRuntimeDirect(t.Context(), filepath.Join(root, "home"), fixture.input, false)
				} else {
					err = process.RunRuntime(t.Context(), filepath.Join(root, "home"), fixture.input)
				}
				if err != nil {
					t.Fatal(err)
				}
				got, err := os.ReadFile(record)
				if err != nil || string(got) != strings.Join(fixture.want, "\n")+"\n" {
					t.Fatalf("child args=%q want=%q err=%v", got, fixture.want, err)
				}
				if !reflect.DeepEqual(fixture.input, original) {
					t.Fatal("launch mutated caller arguments")
				}
			})
		}
	}
}
