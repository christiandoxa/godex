package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

const superResumeAuth04390 = `{"OPENAI_API_KEY":"synthetic-profile-key"}`

type superResumeResult04390 struct {
	Name   string `json:"name"`
	Prodex string `json:"prodex"`
	Godex  string `json:"godex"`
}

type superResumeChild04390 struct {
	Arguments      []string `json:"arguments"`
	Home           string   `json:"home"`
	AuthKind       string   `json:"auth_kind"`
	SessionVisible bool     `json:"session_visible"`
}

func runSuperResumeProbes04390(input options, root string) ([]superResumeResult04390, error) {
	fake, err := writeSuperFakeCodex04390(root)
	if err != nil {
		return nil, err
	}
	var results []superResumeResult04390
	for _, local := range []bool{false, true} {
		name := "legacy_profile_native"
		if local {
			name = "legacy_profile_local"
		}
		result := superResumeResult04390{Name: name}
		for _, product := range []struct{ name, binary string }{{"prodex", input.prodex}, {"godex", input.godex}} {
			directory := filepath.Join(root, name, product.name)
			if err := runSuperResumeProbe04390(product.binary, product.name, directory, fake, local); err != nil {
				return nil, fmt.Errorf("%s Super resume %s: %w", product.name, name, err)
			}
			if product.name == "prodex" {
				result.Prodex = "PASS"
			} else {
				result.Godex = "PASS"
			}
		}
		results = append(results, result)
	}
	return results, nil
}

func runSuperResumeProbe04390(binary, product, root, fake string, local bool) error {
	profile := filepath.Join(root, "profile")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		return err
	}
	auth := filepath.Join(profile, "auth.json")
	if err := os.WriteFile(auth, []byte(superResumeAuth04390), 0o600); err != nil {
		return err
	}
	if err := prepareProfile(binary, product, root, fake); err != nil {
		return err
	}
	rolloutDirectory := filepath.Join(profile, "sessions")
	if err := os.MkdirAll(rolloutDirectory, 0o700); err != nil {
		return err
	}
	metadata, err := json.Marshal(map[string]any{"type": "session_meta", "timestamp": "2026-10-23T00:00:00Z", "payload": map[string]any{
		"id": legacySession04390, "timestamp": "2026-10-23T00:00:00Z", "cwd": root, "source": "cli", "model_provider": "openai",
	}})
	if err != nil {
		return err
	}
	metadata = append(metadata, '\n')
	rollout := filepath.Join(rolloutDirectory, "rollout-2026-10-23T00-00-00-"+legacySession04390+".jsonl")
	if err := os.WriteFile(rollout, metadata, 0o600); err != nil {
		return err
	}
	shared := filepath.Join(root, "shared")
	if err := os.MkdirAll(shared, 0o700); err != nil {
		return err
	}
	record := filepath.Join(root, "child.json")
	env := append(probeEnvironment(root, product, fake, record), "PRODEX_SHARED_CODEX_HOME="+shared)
	arguments := []string{"super", "--no-presidio", "--no-sub-agent", "--skip-quota-check", "--no-auto-rotate", "--profile", "main"}
	if local {
		arguments = append(arguments, "--url", "http://127.0.0.1:11434", "--model", "synthetic-local")
	}
	arguments = append(arguments, "exec", "resume", legacySession04390, "continue")
	if err := runCommand(context.Background(), root, binary, env, arguments...); err != nil {
		return err
	}
	data, err := os.ReadFile(record)
	if err != nil {
		return err
	}
	var child superResumeChild04390
	if err := json.Unmarshal(data, &child); err != nil {
		return err
	}
	if err := verifySuperResumeChild04390(child, profile, local); err != nil {
		return err
	}
	if _, err := os.Stat(child.Home); !os.IsNotExist(err) {
		return fmt.Errorf("Super overlay survived child exit: %v", err)
	}
	for path, want := range map[string]string{auth: superResumeAuth04390, rollout: string(metadata)} {
		if got, err := os.ReadFile(path); err != nil || string(got) != want {
			return fmt.Errorf("Super changed a private profile file: %v", err)
		}
	}
	if err := filepath.WalkDir(shared, func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && filepath.Ext(path) == ".jsonl" {
			return fmt.Errorf("Super copied a legacy rollout into shared storage")
		}
		return err
	}); err != nil {
		return err
	}
	return verifyProfileFiles(root)
}

func verifySuperResumeChild04390(child superResumeChild04390, profile string, local bool) error {
	args := child.Arguments
	execIndex, resumeIndex := slices.Index(args, "exec"), slices.Index(args, "resume")
	if len(args) < 4 || execIndex < 0 || resumeIndex <= execIndex ||
		!slices.Equal(args[len(args)-2:], []string{legacySession04390, "continue"}) ||
		!slices.Contains(args, "--dangerously-bypass-approvals-and-sandbox") {
		return fmt.Errorf("Super changed the native resume command, target, prompt or full access")
	}
	count := 0
	for _, arg := range args {
		if arg == legacySession04390 {
			count++
		}
	}
	wantAuth := "profile"
	if local {
		wantAuth = "synthetic"
	}
	if count != 1 || child.Home == "" || child.Home == profile || child.AuthKind != wantAuth || !child.SessionVisible {
		return fmt.Errorf("Super lost the isolated overlay, credential ownership or single target")
	}
	return nil
}

func writeSuperFakeCodex04390(root string) (string, error) {
	path := filepath.Join(root, "super-codex")
	body := `#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
if '--version' in sys.argv:
    print('codex-cli 0.164.0')
elif 'app-server' in sys.argv[1:]:
    for line in sys.stdin:
        request = json.loads(line)
        if 'id' in request:
            result = {'data': []} if request.get('method') == 'hooks/list' else {}
            print(json.dumps({'id': request['id'], 'result': result}), flush=True)
elif 'exec-server' not in sys.argv[1:]:
    home = Path(os.environ['CODEX_HOME'])
    auth = json.loads((home / 'auth.json').read_text()).get('OPENAI_API_KEY')
    kind = 'profile' if auth == 'synthetic-profile-key' else 'synthetic' if auth in ('prodex-runtime-provider', 'godex-runtime-provider') else 'unexpected'
    session = home / 'sessions' / 'rollout-2026-10-23T00-00-00-01900000-0000-7000-8000-000000000165.jsonl'
    visible = session.is_file() and json.loads(session.read_text().splitlines()[0]).get('payload', {}).get('id') == '01900000-0000-7000-8000-000000000165'
    Path(os.environ['CONTEXT_PARITY_ARGS']).write_text(json.dumps({'arguments': sys.argv[1:], 'home': str(home), 'auth_kind': kind, 'session_visible': visible}))
    print('super-resume-probe-ok')
`
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		return "", err
	}
	return path, nil
}
