package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

const nativeSession04390 = "01900000-0000-7000-8000-000000000164"

type nativeArgumentProbe04390 struct {
	name        string
	input, want []string
}

type nativeArgumentResult04390 struct {
	Name   string   `json:"name"`
	Prodex []string `json:"prodex"`
	Godex  []string `json:"godex"`
}

func nativeArgumentProbes04390() []nativeArgumentProbe04390 {
	probes := []nativeArgumentProbe04390{
		{"root_exec", []string{"-c", `model_reasoning_effort="low"`, "exec", "--config", `model_reasoning_effort="high"`, "hello"}, []string{"exec", "-c", `model_reasoning_effort="low"`, "--config", `model_reasoning_effort="high"`, "hello"}},
		{"root_exec_resume", []string{"-c", `model_reasoning_effort="low"`, "exec", "--config", `model_reasoning_effort="high"`, "resume", "-c", `model_reasoning_effort="medium"`, nativeSession04390, "continue without replaying completed tools"}, []string{"exec", "resume", "-c", `model_reasoning_effort="low"`, "--config", `model_reasoning_effort="high"`, "-c", `model_reasoning_effort="medium"`, nativeSession04390, "continue without replaying completed tools"}},
		{"inline_config_forms", []string{`--config=model_reasoning_effort="low"`, "exec", `-cmodel_reasoning_effort="high"`, "resume", "--last", "hello"}, []string{"exec", "resume", `--config=model_reasoning_effort="low"`, `-cmodel_reasoning_effort="high"`, "--last", "hello"}},
		{"literal_tail", []string{"-c", "model_context_window=500000", "exec", "--", "--config", "literal=prompt"}, []string{"exec", "-c", "model_context_window=500000", "--", "--config", "literal=prompt"}},
	}
	for index, prompt := range []string{"/model gpt-5.6 high", "/permissions read-only", "/presets"} {
		probes = append(probes, nativeArgumentProbe04390{fmt.Sprintf("native_inline_%d", index),
			[]string{"resume", nativeSession04390, prompt},
			[]string{"-c", "disable_paste_burst=true", "resume", nativeSession04390, prompt}})
	}
	return probes
}

func runNativeArgumentProbes04390(input options, prodexRoot, godexRoot, fake string) ([]nativeArgumentResult04390, error) {
	var results []nativeArgumentResult04390
	for _, probe := range nativeArgumentProbes04390() {
		result := nativeArgumentResult04390{Name: probe.name}
		for _, product := range []struct{ name, binary, root string }{
			{"prodex", input.prodex, prodexRoot}, {"godex", input.godex, godexRoot},
		} {
			args, err := runNativeArgumentProbe04390(product.binary, product.name, product.root, fake, probe)
			if err != nil {
				return nil, fmt.Errorf("%s native arguments %s: %w", product.name, probe.name, err)
			}
			if product.name == "prodex" {
				result.Prodex = args
			} else {
				result.Godex = args
			}
		}
		results = append(results, result)
	}
	return results, nil
}

func runNativeArgumentProbe04390(binary, name, root, fake string, probe nativeArgumentProbe04390) ([]string, error) {
	shared := filepath.Join(root, "native-shared")
	rollouts := filepath.Join(shared, "sessions", "2026", "10", "23")
	if err := os.MkdirAll(rollouts, 0o700); err != nil {
		return nil, err
	}
	metadata, err := json.Marshal(map[string]any{"type": "session_meta", "timestamp": "2026-10-23T00:00:00Z", "payload": map[string]any{
		"id": nativeSession04390, "timestamp": "2026-10-23T00:00:00Z", "cwd": root, "source": "cli", "model_provider": "openai",
	}})
	if err != nil {
		return nil, err
	}
	path := filepath.Join(rollouts, "rollout-2026-10-23T00-00-00-"+nativeSession04390+".jsonl")
	if err := os.WriteFile(path, append(metadata, '\n'), 0o600); err != nil {
		return nil, err
	}
	record := filepath.Join(root, probe.name+"-native-args")
	env := append(probeEnvironment(root, name, fake, record), "PRODEX_SHARED_CODEX_HOME="+shared)
	args := append([]string{"run", "--skip-quota-check", "--no-auto-rotate", "--profile", "main"}, probe.input...)
	if err := runCommand(context.Background(), root, binary, env, args...); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(record)
	if err != nil {
		return nil, err
	}
	got := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if err := verifyNativeArguments04390(got, probe.want); err != nil {
		return nil, err
	}
	home, err := os.ReadFile(record + ".home")
	if err != nil || string(home) != filepath.Join(root, "profile") {
		return nil, fmt.Errorf("native child lost profile home: %q error=%v", home, err)
	}
	if err := verifyProfileFiles(root); err != nil {
		return nil, err
	}
	return got, nil
}

func verifyNativeArguments04390(got, want []string) error {
	if !reflect.DeepEqual(got, want) {
		return fmt.Errorf("native child args=%q want=%q", got, want)
	}
	return nil
}
