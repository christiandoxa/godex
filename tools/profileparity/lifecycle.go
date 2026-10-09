package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"time"
)

type stateProjection struct {
	Active string
	Names  []string
}
type profileFixture struct {
	name         string
	binary       string
	home         string
	configHome   string
	externalHome string
	env          []string
}
type lifecycleStep struct {
	name            string
	command         []string
	wantExitSuccess bool
	state           stateProjection
	alphaHome       bool
	betaHome        bool
	outputContains  string
}

func profileSteps() []lifecycleStep {
	return []lifecycleStep{
		{"initial_list", []string{"profile", "list"}, true, stateProjection{}, false, false, "No profiles configured"},
		{"reject_unsupported_gateway_provider", []string{"gateway", "--provider", "openai"}, false, stateProjection{}, false, false, "invalid --provider"},
		{"reject_profileless_raw_key_gateway", []string{
			"gateway", "--provider", "deepseek", "--api-key", "synthetic-gateway-credential",
			"--base-url", "http://127.0.0.1:1/v1", "--listen", "127.0.0.1:0",
		}, false, stateProjection{}, false, false, "no active profile selected"},
		{"create_alpha", []string{"profile", "add", "alpha"}, true, stateProjection{"alpha", []string{"alpha"}}, true, false, "alpha"},
		{"duplicate_alpha_rejected", []string{"profile", "add", "alpha"}, false, stateProjection{"alpha", []string{"alpha"}}, true, false, ""},
		{"create_beta", []string{"profile", "add", "beta"}, true, stateProjection{"alpha", []string{"alpha", "beta"}}, true, true, "beta"},
		{"list_two", []string{"profile", "list"}, true, stateProjection{"alpha", []string{"alpha", "beta"}}, true, true, "alpha"},
		{"activate_beta", []string{"profile", "use", "--profile", "beta"}, true, stateProjection{"beta", []string{"alpha", "beta"}}, true, true, "beta"},
		{"read_current_beta", []string{"current"}, true, stateProjection{"beta", []string{"alpha", "beta"}}, true, true, "beta"},
		{"reject_unknown_selection", []string{"profile", "use", "--profile", "missing"}, false, stateProjection{"beta", []string{"alpha", "beta"}}, true, true, ""},
		{"remove_alpha_keep_home", []string{"profile", "remove", "alpha"}, true, stateProjection{"beta", []string{"beta"}}, true, true, "alpha"},
		{"restart_read_beta", []string{"current"}, true, stateProjection{"beta", []string{"beta"}}, true, true, "beta"},
		{"delete_beta_home", []string{"profile", "remove", "--delete-home", "beta"}, true, stateProjection{"", []string{}}, true, false, "beta"},
		{"list_empty", []string{"profile", "list"}, true, stateProjection{}, true, false, "No profiles configured"},
		{"reject_missing_removal", []string{"profile", "remove", "missing"}, false, stateProjection{}, true, false, ""},
		{"register_external", []string{"profile", "add", "external", "--codex-home", "@EXTERNAL_HOME@"}, true, stateProjection{"external", []string{"external"}}, true, false, "external"},
		{"list_external", []string{"profile", "list"}, true, stateProjection{"external", []string{"external"}}, true, false, "external"},
		{"reject_external_delete", []string{"profile", "remove", "--delete-home", "external"}, false, stateProjection{"external", []string{"external"}}, true, false, "refusing to delete external"},
		{"restart_read_external", []string{"current"}, true, stateProjection{"external", []string{"external"}}, true, false, "external"},
		{"remove_external_keep_home", []string{"profile", "remove", "external"}, true, stateProjection{}, true, false, "external"},
		{"list_empty_after_external", []string{"profile", "list"}, true, stateProjection{}, true, false, "No profiles configured"},
	}
}

func checkProfileLifecycle(root string, opts cliOptions) ([]stepResult, error) {
	fixtures := make([]profileFixture, 0, 2)
	for _, item := range []struct{ name, binary string }{
		{"prodex", opts.prodex}, {"godex", opts.godex},
	} {
		directory := filepath.Join(root, item.name)
		userHome := filepath.Join(directory, "user")
		configHome := filepath.Join(directory, item.name)
		externalHome := filepath.Join(directory, "external")
		if err := os.MkdirAll(userHome, 0o700); err != nil {
			return nil, err
		}
		if err := os.Mkdir(externalHome, 0o700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(externalHome, "untouched.txt"), []byte("keep-external-home"), 0o600); err != nil {
			return nil, err
		}
		variables := []string{
			"HOME=" + userHome, "PRODEX_HOME=" + filepath.Join(directory, "prodex"),
			"GODEX_HOME=" + filepath.Join(directory, "godex"),
			"CODEX_HOME=" + filepath.Join(userHome, ".codex"),
			"XDG_CONFIG_HOME=" + filepath.Join(userHome, ".config"),
			"XDG_DATA_HOME=" + filepath.Join(userHome, ".local", "share"),
			"PATH=" + os.Getenv("PATH"),
			"NO_COLOR=1", "CI=1",
			"PRODEX_NO_UPDATE_CHECK=1", "GODEX_NO_UPDATE_CHECK=1",
			// A profile test never needs network or real provider credentials.
			"HTTP_PROXY=http://127.0.0.1:1",
			"HTTPS_PROXY=http://127.0.0.1:1",
			"ALL_PROXY=http://127.0.0.1:1",
		}
		fixtures = append(fixtures, profileFixture{
			name: item.name, binary: item.binary, home: userHome,
			configHome: configHome, externalHome: externalHome, env: variables,
		})
	}

	results := make([]stepResult, 0, len(profileSteps()))
	for _, step := range profileSteps() {
		observed := make([]stateProjection, 0, 2)
		codes := make([]int, 0, 2)
		for _, item := range fixtures {
			code, output, err := invokeProfileCommand(item, step.command)
			if err != nil {
				return nil, fmt.Errorf("%s %s: %w", item.name, step.name, err)
			}
			if gotSuccess := code == 0; gotSuccess != step.wantExitSuccess {
				return nil, fmt.Errorf("%s %s returned exit %d: expected success=%t; stdout/stderr=%q",
					item.name, step.name, code, step.wantExitSuccess, output)
			}
			if step.outputContains != "" &&
				!strings.Contains(strings.ToLower(output), strings.ToLower(step.outputContains)) {
				return nil, fmt.Errorf("%s %s response lacked expected marker", item.name, step.name)
			}
			projection, err := readAndValidateState(item)
			if err != nil {
				return nil, fmt.Errorf("%s %s state: %w", item.name, step.name, err)
			}
			if !equivalentStateProjection(projection, step.state) {
				return nil, fmt.Errorf("%s %s state drift: active=%q profiles=%v, want active=%q profiles=%v",
					item.name, step.name, projection.Active, projection.Names, step.state.Active, step.state.Names)
			}
			if err := checkManagedHomes(item, step.alphaHome, step.betaHome); err != nil {
				return nil, fmt.Errorf("%s %s homes: %w", item.name, step.name, err)
			}
			if err := checkExternalHome(item); err != nil {
				return nil, fmt.Errorf("%s %s external home: %w", item.name, step.name, err)
			}
			observed = append(observed, projection)
			codes = append(codes, code)
		}
		if !equivalentStateProjection(observed[0], observed[1]) || codes[0] != codes[1] {
			return nil, fmt.Errorf("%s: Prodex and Godex projected states or exit codes differ", step.name)
		}
		results = append(results, stepResult{
			Name: step.name, Active: step.state.Active,
			Profiles:   append([]string{}, step.state.Names...),
			ProdexExit: codes[0], GodexExit: codes[1],
		})
	}
	return results, nil
}

func invokeProfileCommand(fixture profileFixture, args []string) (int, string, error) {
	replaced := append([]string(nil), args...)
	for i, arg := range replaced {
		if arg == "@EXTERNAL_HOME@" {
			replaced[i] = fixture.externalHome
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, fixture.binary, replaced...)
	command.Dir = fixture.home
	command.Env = fixture.env
	output, err := command.CombinedOutput()
	if len(output) > 16<<10 {
		return 0, "", errors.New("unexpectedly large CLI response")
	}
	if ctx.Err() != nil {
		return 0, "", fmt.Errorf("CLI deadline: %w", ctx.Err())
	}
	if err == nil {
		return 0, string(output), nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), string(output), nil
	}
	return 0, "", err
}

func equivalentStateProjection(left, right stateProjection) bool {
	if left.Active != right.Active {
		return false
	}
	return reflect.DeepEqual(append([]string{}, left.Names...), append([]string{}, right.Names...))
}
func readAndValidateState(fixture profileFixture) (stateProjection, error) {
	name := "profiles.json"
	if fixture.name == "prodex" {
		name = "state.json"
	}
	path := filepath.Join(fixture.configHome, name)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return stateProjection{}, nil
	}
	if err != nil {
		return stateProjection{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > 2<<20 ||
		(runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0) {
		return stateProjection{}, errors.New("persisted profile state file is unsafe")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return stateProjection{}, err
	}
	return decodeProfileState(fixture.name, raw, fixture.configHome)
}

func decodeProfileState(product string, raw []byte, root string) (stateProjection, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return stateProjection{}, err
	}
	if envelope == nil {
		return stateProjection{}, errors.New("missing state envelope")
	}
	allowed := map[string]bool{"version": true, "active_profile": true, "profiles": true}
	if product == "prodex" {
		allowed = map[string]bool{
			"schema_version": true, "active_profile": true, "profiles": true,
			"last_run_selected_at": true, "response_profile_bindings": true,
			"session_profile_bindings": true,
		}
	}
	for key := range envelope {
		if !allowed[key] {
			return stateProjection{}, fmt.Errorf("unreviewed state field %s", key)
		}
	}
	versionKey, version := "version", 0
	if product == "prodex" {
		versionKey = "schema_version"
	}
	if err := json.Unmarshal(envelope[versionKey], &version); err != nil || version != 1 {
		return stateProjection{}, errors.New("invalid profile state schema version")
	}
	var active string
	if len(envelope["active_profile"]) > 0 && string(envelope["active_profile"]) != "null" {
		if err := json.Unmarshal(envelope["active_profile"], &active); err != nil {
			return stateProjection{}, errors.New("invalid active profile")
		}
	}
	result := stateProjection{Active: active}
	if product == "prodex" {
		var entries map[string]json.RawMessage
		if err := json.Unmarshal(envelope["profiles"], &entries); err != nil || entries == nil {
			return stateProjection{}, errors.New("Prodex profile map missing")
		}
		for name, rawEntry := range entries {
			if err := verifyEntry(name, rawEntry, root); err != nil {
				return stateProjection{}, err
			}
			result.Names = append(result.Names, name)
		}
		for _, field := range []string{"response_profile_bindings", "session_profile_bindings"} {
			var bindings map[string]json.RawMessage
			if err := json.Unmarshal(envelope[field], &bindings); err != nil || bindings == nil || len(bindings) > 0 {
				return stateProjection{}, fmt.Errorf("unexpected persistent %s in credential-free fixture", field)
			}
		}
		if lastRunRaw, exists := envelope["last_run_selected_at"]; exists {
			var lastRun map[string]json.RawMessage
			if err := json.Unmarshal(lastRunRaw, &lastRun); err != nil || lastRun == nil {
				return stateProjection{}, errors.New("invalid last-run selection metadata")
			}
			for name, value := range lastRun {
				if !containsName(result.Names, name) {
					return stateProjection{}, errors.New("stale removed profile selection metadata")
				}
				var epoch int64
				if err := json.Unmarshal(value, &epoch); err != nil || epoch <= 0 {
					return stateProjection{}, errors.New("invalid last-run selection timestamp")
				}
			}
		}
	} else if product == "godex" {
		var entries []map[string]json.RawMessage
		if err := json.Unmarshal(envelope["profiles"], &entries); err != nil || entries == nil {
			return stateProjection{}, errors.New("Godex profile list missing")
		}
		for _, entry := range entries {
			var name string
			if json.Unmarshal(entry["name"], &name) != nil {
				return stateProjection{}, errors.New("invalid Godex profile name")
			}
			encoded, _ := json.Marshal(entry)
			if err := verifyEntry(name, encoded, root); err != nil {
				return stateProjection{}, err
			}
			result.Names = append(result.Names, name)
		}
	} else {
		return stateProjection{}, errors.New("unrecognized product identity")
	}
	sort.Strings(result.Names)
	for i := 1; i < len(result.Names); i++ {
		if result.Names[i] == result.Names[i-1] {
			return stateProjection{}, errors.New("duplicate profile entry")
		}
	}
	if active != "" && !containsName(result.Names, active) {
		return stateProjection{}, errors.New("active profile missing from persisted list")
	}
	return result, nil
}

func containsName(names []string, target string) bool {
	for _, name := range names {
		if name == target {
			return true
		}
	}
	return false
}
func verifyEntry(name string, raw []byte, root string) error {
	if name != "alpha" && name != "beta" && name != "external" {
		return errors.New("unexpected persisted profile name")
	}
	var entry struct {
		CodexHome string `json:"codex_home"`
		Managed   bool   `json:"managed"`
		Provider  struct {
			Kind string `json:"provider_kind"`
		} `json:"provider"`
	}
	if err := json.Unmarshal(raw, &entry); err != nil {
		return err
	}
	expected := filepath.Join(root, "profiles", name)
	wantManaged := true
	if name == "external" {
		expected = filepath.Join(filepath.Dir(root), "external")
		wantManaged = false
	}
	if entry.Managed != wantManaged || entry.Provider.Kind != "openai" ||
		filepath.Clean(entry.CodexHome) != expected {
		return fmt.Errorf("profile %s metadata disagrees with fixture", name)
	}
	return nil
}
func checkManagedHomes(fixture profileFixture, alpha, beta bool) error {
	profilesPath := filepath.Join(fixture.configHome, "profiles")
	rootInfo, err := os.Lstat(profilesPath)
	if errors.Is(err, os.ErrNotExist) && !alpha && !beta {
		return nil
	}
	if err != nil {
		return err
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 ||
		(runtime.GOOS != "windows" && rootInfo.Mode().Perm()&0o077 != 0) {
		return errors.New("profiles root is not a private real directory")
	}
	entries, err := os.ReadDir(profilesPath)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() != "alpha" && entry.Name() != "beta" {
			return fmt.Errorf("unrecognized profile home entry %q", entry.Name())
		}
	}
	for _, item := range []struct {
		name string
		want bool
	}{{"alpha", alpha}, {"beta", beta}} {
		path := filepath.Join(fixture.configHome, "profiles", item.name)
		info, err := os.Lstat(path)
		if !item.want {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return err
			}
			return fmt.Errorf("profile home %s survived deletion unexpectedly", item.name)
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 ||
			(runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0) {
			return fmt.Errorf("profile home %s is not a private real directory", item.name)
		}
		if _, err := os.Lstat(filepath.Join(path, "auth.json")); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("credential-free fixture generated unexpected authentication material")
		}
	}
	return nil
}

// Registration of a user-owned CODEX_HOME is metadata-only. Even when asked
// to delete a profile, the tool must preserve its external directory, files,
// and private permissions; neither application may remove it as managed state.
func checkExternalHome(fixture profileFixture) error {
	root, err := os.Lstat(fixture.externalHome)
	if err != nil {
		return err
	}
	if !root.IsDir() || root.Mode()&os.ModeSymlink != 0 ||
		(runtime.GOOS != "windows" && root.Mode().Perm()&0o077 != 0) {
		return errors.New("external profile home became unsafe")
	}
	data, err := os.ReadFile(filepath.Join(fixture.externalHome, "untouched.txt"))
	if err != nil {
		return err
	}
	if string(data) != "keep-external-home" {
		return errors.New("external profile contents were modified")
	}
	return nil
}
