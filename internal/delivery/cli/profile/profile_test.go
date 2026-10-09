package profile

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	accountrepo "github.com/christiandoxa/godex/internal/repository/account"
	profilerepo "github.com/christiandoxa/godex/internal/repository/profile"
	profileusecase "github.com/christiandoxa/godex/internal/usecase/profile"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

func newProfileCatalog(t *testing.T) (*profileusecase.Catalog, string) {
	t.Helper()
	root := t.TempDir()
	current := filepath.Join(t.TempDir(), "current-codex")
	if err := os.Mkdir(current, 0o700); err != nil {
		t.Fatal(err)
	}
	profiles := profilerepo.NewStore(root)
	accounts := accountrepo.NewFileStore(root)
	return profileusecase.NewCatalog(profiles, accounts, current), current
}

func TestProfileCommandsAddListCurrentUseAndRemove(t *testing.T) {
	catalog, _ := newProfileCatalog(t)
	var output bytes.Buffer
	if err := Run(context.Background(), catalog, &output, []string{"add", "work"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Added profile work.") || !strings.Contains(output.String(), "Storage: managed") {
		t.Fatalf("add output = %q", output.String())
	}
	output.Reset()
	if err := Run(context.Background(), catalog, &output, []string{"list"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "work") || !strings.Contains(output.String(), "openai") {
		t.Fatalf("list output = %q", output.String())
	}
	output.Reset()
	if err := Run(context.Background(), catalog, &output, []string{"current"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Profile: work") {
		t.Fatalf("current output = %q", output.String())
	}
	output.Reset()
	if err := Run(context.Background(), catalog, &output, []string{"use", "--profile", "work"}); err != nil {
		t.Fatal(err)
	}
	if output.String() != "Active profile: work\n" {
		t.Fatalf("use output = %q", output.String())
	}
	output.Reset()
	if err := Run(context.Background(), catalog, &output, []string{"remove", "work"}); err != nil {
		t.Fatal(err)
	}
	if output.String() != "Removed profile work.\n" {
		t.Fatalf("remove output = %q", output.String())
	}
}

func TestProfileCurrentReportsEmptySelection(t *testing.T) {
	catalog, _ := newProfileCatalog(t)
	var output bytes.Buffer
	if err := Run(context.Background(), catalog, &output, []string{"current"}); err != nil {
		t.Fatalf("current without profiles: %v", err)
	}
	if output.String() != "No active profile.\n" {
		t.Fatalf("empty current output = %q", output.String())
	}
}

func TestProfileCopyCurrentAndExternalDeletionSafety(t *testing.T) {
	catalog, current := newProfileCatalog(t)
	if err := os.WriteFile(filepath.Join(current, "config.toml"), []byte("model = \"synthetic\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(current, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	var output bytes.Buffer
	if err := Run(context.Background(), catalog, &output, []string{"add", "copy", "--copy-current"}); err != nil {
		t.Fatal(err)
	}
	report, err := catalog.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(report.Profile.CodexHome, "config.toml")); err != nil {
		t.Fatalf("copied current home: %v", err)
	}

	external := t.TempDir()
	if runtime.GOOS != "windows" {
		if err := os.Chmod(external, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	output.Reset()
	if err := Run(context.Background(), catalog, &output, []string{"add", "external", "--codex-home", external}); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), catalog, &bytes.Buffer{}, []string{"remove", "external", "--delete-home"}); err == nil {
		t.Fatal("external profile home deletion unexpectedly accepted")
	}
}

func TestProfileCommandsRejectInvalidArguments(t *testing.T) {
	catalog, _ := newProfileCatalog(t)
	for _, arguments := range [][]string{
		nil,
		{"add"},
		{"add", "work", "--copy-current", "--copy-from", "/tmp/source"},
		{"remove"},
		{"use"},
		{"unknown"},
	} {
		if err := Run(context.Background(), catalog, &bytes.Buffer{}, arguments); err == nil {
			t.Fatalf("arguments %#v unexpectedly accepted", arguments)
		}
	}
}

func TestProfileExportArgumentParsing(t *testing.T) {
	t.Setenv(exportPasswordEnv, "bundle-test-password")
	request, err := parseExport([]string{"-p", "one", "--profile=two", "--password-protect", "bundle.json"})
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Profiles) != 2 || request.Profiles[0] != "one" || request.Profiles[1] != "two" || request.Password == "" || request.OutputPath != "bundle.json" {
		t.Fatalf("request = %#v", request)
	}
	plain, err := parseExport([]string{"--no-password", "bundle.json"})
	if err != nil || plain.Password != "" {
		t.Fatalf("plain = %#v, err = %v", plain, err)
	}
	for _, arguments := range [][]string{
		{"bundle.json"},
		{"--password-protect", "--no-password", "bundle.json"},
		{"--profile", "--no-password", "bundle.json"},
		{"--unknown", "--no-password"},
	} {
		if _, err := parseExport(arguments); err == nil {
			t.Fatalf("arguments %#v unexpectedly accepted", arguments)
		}
	}
}

func TestPasswordModeTUIUsesProdexDefaults(t *testing.T) {
	for _, test := range []struct {
		key     tea.KeyMsg
		protect bool
	}{
		{tea.KeyMsg{Type: tea.KeyEnter}, true},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}, true},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}}, false},
		{tea.KeyMsg{Type: tea.KeyEsc}, false},
	} {
		updated, command := (passwordModeModel{}).Update(test.key)
		model := updated.(passwordModeModel)
		if command == nil || !model.done || model.protect != test.protect {
			t.Fatalf("key %q = %+v, command=%v", test.key.String(), model, command)
		}
	}
	view := (passwordModeModel{}).View()
	if !strings.Contains(view, "Password-protect") || !strings.Contains(view, "y/enter protect") {
		t.Fatalf("mode view = %q", view)
	}
}

func TestPasswordEntryTUIAlwaysMasksInput(t *testing.T) {
	model := passwordEntryModel{title: "Profile Export", label: "Export password", detail: "Enter password"}
	for _, ch := range "super-secret-value" {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = updated.(passwordEntryModel)
	}
	view := model.View()
	if strings.Contains(view, "super-secret-value") || !strings.Contains(view, strings.Repeat("*", len([]rune("super-secret-value")))) {
		t.Fatalf("password view leaked or failed to mask: %q", view)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	model = updated.(passwordEntryModel)
	if len(model.password) != len([]rune("super-secret-value"))-1 {
		t.Fatalf("backspace length = %d", len(model.password))
	}
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(passwordEntryModel)
	if command == nil || !model.accepted || model.cancelled {
		t.Fatalf("enter model = %+v command=%v", model, command)
	}
	cancelled, command := (passwordEntryModel{}).Update(tea.KeyMsg{Type: tea.KeyEsc})
	if command == nil || !cancelled.(passwordEntryModel).cancelled {
		t.Fatal("escape did not cancel password entry")
	}
}

func TestProfilePasswordPoliciesRemainFailClosedWithoutTTYOrEnv(t *testing.T) {
	t.Setenv(exportPasswordEnv, "")
	t.Setenv(importPasswordEnv, "")
	if _, err := parseExport([]string{"bundle.json"}); err == nil || !strings.Contains(err.Error(), "non-interactive") {
		t.Fatalf("missing export mode error = %v", err)
	}
	if _, err := parseExport([]string{"--password-protect", "bundle.json"}); err == nil || !strings.Contains(err.Error(), exportPasswordEnv) {
		t.Fatalf("missing export password error = %v", err)
	}
	if _, err := resolveImportPassword(context.Background()); err == nil || !strings.Contains(err.Error(), importPasswordEnv) {
		t.Fatalf("missing import password error = %v", err)
	}
}

func TestImportPasswordRetryOnlyRecognizesPasswordErrors(t *testing.T) {
	if !importRequiresPassword(errors.New("profile export password must contain bytes")) {
		t.Fatal("password error was not recognized")
	}
	if importRequiresPassword(errors.New("failed to parse bundle")) {
		t.Fatal("unrelated import error unexpectedly requested a password")
	}
}

func TestProfileImportArgumentParsingSupportsBuiltInSources(t *testing.T) {
	options, err := parseImport([]string{"claude", "--name", "work", "--activate", "--insecure"})
	if err != nil {
		t.Fatal(err)
	}
	if options.path != "claude" || options.name != "work" || !options.activate || !options.insecure {
		t.Fatalf("options = %+v", options)
	}
	for _, arguments := range [][]string{
		nil,
		{"claude", "other"},
		{"claude", "--name"},
		{"claude", "--unknown"},
	} {
		if _, err := parseImport(arguments); err == nil {
			t.Fatalf("arguments %#v unexpectedly accepted", arguments)
		}
	}
}

func TestBuiltInImportSourceDoesNotShadowExistingFile(t *testing.T) {
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if builtinImportSource("claude") != true {
		t.Fatal("missing claude path should resolve built-in source")
	}
	if err := os.WriteFile("claude", []byte("bundle"), 0o600); err != nil {
		t.Fatal(err)
	}
	if builtinImportSource("claude") {
		t.Fatal("existing file named claude was shadowed by built-in source")
	}
}

func TestBuiltinImportResultRendersWarning(t *testing.T) {
	var output bytes.Buffer
	err := writeBuiltinImportResult(&output, profilemodel.BuiltinImportResult{
		Profile: "kiro-main", Provider: "kiro", Updated: true, Active: true,
		Warning: "Kiro model catalog refresh failed; re-import this profile to retry.",
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := output.String()
	for _, expected := range []string{"Updated kiro profile", "Active: true", "Warning: Kiro model catalog refresh failed"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("output missing %q: %q", expected, rendered)
		}
	}
}
