package profile

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	accountrepo "github.com/christiandoxa/godex/internal/repository/account"
	profilerepo "github.com/christiandoxa/godex/internal/repository/profile"
	profileusecase "github.com/christiandoxa/godex/internal/usecase/profile"
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
