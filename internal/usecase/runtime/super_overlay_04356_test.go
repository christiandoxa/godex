package runtime

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestProdex04356SuperOverlayCopiesMetadataAndSharesState(t *testing.T) {
	root := filepath.Join(t.TempDir(), "profiles")
	base := filepath.Join(t.TempDir(), "base")
	if err := os.MkdirAll(filepath.Join(base, "cache", "codex_apps_tools"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "packages", "standalone"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "auth.json"), []byte("credential"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "config.toml"), []byte("model = \"gpt-5.4\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "history.jsonl"), []byte("history\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "sessions", "thread.jsonl"), []byte("session\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "state_1.sqlite"), []byte("state"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "packages", "standalone", "binary"), []byte("managed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "cache", "codex_apps_tools", "catalog"), []byte("cache"), 0o600); err != nil {
		t.Fatal(err)
	}

	overlay, err := PrepareSuperOverlay(root, base)
	if err != nil {
		t.Fatal(err)
	}
	home := overlay.Home
	defer overlay.Close()

	if got, err := os.ReadFile(filepath.Join(home, "auth.json")); err != nil || string(got) != "credential" {
		t.Fatalf("overlay auth = %q err=%v", got, err)
	}
	if _, err := os.Stat(filepath.Join(home, "packages")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("managed packages copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "cache", "codex_apps_tools")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Codex apps cache retained: %v", err)
	}
	for _, relative := range []string{"history.jsonl", "sessions", "archived_sessions", "attachments", "image_attachments", "state_1.sqlite"} {
		info, err := os.Lstat(filepath.Join(home, relative))
		if err != nil {
			t.Fatalf("%s missing: %v", relative, err)
		}
		if runtime.GOOS != "windows" && info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("%s is not shared by symlink: mode=%v", relative, info.Mode())
		}
	}
	if err := os.WriteFile(filepath.Join(home, "history.jsonl"), []byte("updated\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(base, "history.jsonl")); err != nil || string(got) != "updated\n" {
		t.Fatalf("shared history = %q err=%v", got, err)
	}

	configBytes, err := os.ReadFile(filepath.Join(home, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := toml.Unmarshal(configBytes, &config); err != nil {
		t.Fatal(err)
	}
	if config["approval_policy"] != "never" || config["sandbox_mode"] != "danger-full-access" || config["model"] != "gpt-5.4" {
		t.Fatalf("overlay config = %#v", config)
	}
	baseConfig, err := os.ReadFile(filepath.Join(base, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(baseConfig), "danger-full-access") || strings.Contains(string(baseConfig), "approval_policy") {
		t.Fatalf("base config was mutated: %s", baseConfig)
	}

	if err := overlay.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(home); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("overlay survived cleanup: %v", err)
	}
	if _, err := os.Stat(base); err != nil {
		t.Fatalf("base disappeared after cleanup: %v", err)
	}
}

func TestProdex04356SuperOverlayRejectsSymlinkRoot(t *testing.T) {
	target := t.TempDir()
	root := filepath.Join(t.TempDir(), "profiles-link")
	if err := os.Symlink(target, root); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	base := t.TempDir()
	if _, err := PrepareSuperOverlay(root, base); err == nil || !strings.Contains(err.Error(), "must not be a symbolic link") {
		t.Fatalf("symlink root = %v", err)
	}
}

func TestProdex04356SuperOverlayCopiesOnlyInRootSymlinkTargets(t *testing.T) {
	root := filepath.Join(t.TempDir(), "profiles")
	base := t.TempDir()
	inside := filepath.Join(base, "inside.txt")
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(inside, []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(inside, filepath.Join(base, "inside-link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(base, "outside-link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	overlay, err := PrepareSuperOverlay(root, base)
	if err != nil {
		t.Fatal(err)
	}
	defer overlay.Close()
	if got, err := os.ReadFile(filepath.Join(overlay.Home, "inside-link")); err != nil || string(got) != "inside" {
		t.Fatalf("in-root symlink copy = %q err=%v", got, err)
	}
	if _, err := os.Lstat(filepath.Join(overlay.Home, "outside-link")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("escaping symlink copied: %v", err)
	}
}

func TestProdex04356SuperOverlayPermissionsArePrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode permissions are Unix-specific")
	}
	root := filepath.Join(t.TempDir(), "profiles")
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "auth.json"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	overlay, err := PrepareSuperOverlay(root, base)
	if err != nil {
		t.Fatal(err)
	}
	defer overlay.Close()
	if mode, err := superOverlayMode(root); err != nil || mode.Perm() != 0o700 {
		t.Fatalf("root mode = %v err=%v", mode, err)
	}
	if mode, err := superOverlayMode(overlay.Home); err != nil || mode.Perm() != 0o700 {
		t.Fatalf("overlay mode = %v err=%v", mode, err)
	}
	if mode, err := superOverlayMode(filepath.Join(overlay.Home, "auth.json")); err != nil || mode.Perm() != 0o600 {
		t.Fatalf("file mode = %v err=%v", mode, err)
	}
}
