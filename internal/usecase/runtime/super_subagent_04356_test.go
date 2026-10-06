package runtime

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/lockfile"
)

func TestProdex04356SuperSubAgentOverlayWritesLauncherSlotsAndAgentsBlock(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "AGENTS.md"), []byte("base instructions\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := SuperSubAgentConfig{
		Provider:             "deepseek",
		Model:                "deepseek-chat",
		Effort:               "high",
		MaxConcurrency:       3,
		MaxConcurrencySource: "custom",
		PresidioEnabled:      true,
		RequiredTools:        []string{"rtk", "presidio"},
	}
	if err := PrepareSuperSubAgentOverlay(home, config); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(home, "sub-agent-launch.json"))
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err := json.Unmarshal(content, &spec); err != nil {
		t.Fatal(err)
	}
	if spec["provider"] != "deepseek" || spec["model"] != "deepseek-chat" ||
		spec["effort"] != "high" || spec["presidio-enabled"] != true ||
		spec["recursion-marker"] != "GODEX_SUB_AGENT" ||
		spec["task-max-bytes"] != float64(65_536) {
		t.Fatalf("launcher spec = %#v", spec)
	}
	maximum := spec["max-concurrency"].(map[string]any)
	if maximum["value"] != float64(3) || maximum["source"] != "custom" {
		t.Fatalf("max concurrency = %#v", maximum)
	}
	required := spec["required-tools"].([]any)
	if len(required) != 2 || required[0] != "rtk" || required[1] != "presidio" {
		t.Fatalf("required tools = %#v", required)
	}
	executable, _ := spec["executable"].(string)
	if !filepath.IsAbs(executable) {
		t.Fatalf("launcher executable = %q", executable)
	}
	if spec["slot-dir"] != filepath.Join(home, "sub-agent-slots") ||
		spec["task-dir"] != filepath.Join(home, "sub-agent-tasks") {
		t.Fatalf("launcher directories = %#v", spec)
	}

	for index := 0; index < 3; index++ {
		path := filepath.Join(home, "sub-agent-slots", "slot-0"+string(rune('0'+index))+".lock")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("slot %d missing: %v", index, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "sub-agent-slots", "slot-03.lock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected slot 03: %v", err)
	}
	if goruntime.GOOS != "windows" {
		for _, path := range []string{
			filepath.Join(home, "sub-agent-tasks"),
			filepath.Join(home, "sub-agent-slots"),
		} {
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o700 {
				t.Fatalf("%s mode = %o", path, info.Mode().Perm())
			}
		}
		info, err := os.Stat(filepath.Join(home, "sub-agent-launch.json"))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("launcher config mode = %o", info.Mode().Perm())
		}
	}

	instructions, err := os.ReadFile(filepath.Join(home, "SUB_AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(instructions)
	for _, want := range []string{
		"Maximum active sub-agents: 3 (custom)",
		"Inherited Presidio: enabled",
		"Inherited required tools: rtk, presidio",
		"__sub-agent-exec",
		"GODEX_SUB_AGENT=1",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("sub-agent instructions missing %q:\n%s", want, text)
		}
	}
	if count := numberedSubAgentRules(text); count != 18 {
		t.Fatalf("numbered rules = %d, want 18", count)
	}

	agents, err := os.ReadFile(filepath.Join(home, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(agents), "base instructions") ||
		strings.Count(string(agents), "<!-- GODEX SUB-AGENT BEGIN -->") != 1 {
		t.Fatalf("AGENTS.md = %s", agents)
	}

	if err := PrepareSuperSubAgentOverlay(home, config); err != nil {
		t.Fatal(err)
	}
	agents, err = os.ReadFile(filepath.Join(home, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(agents), "<!-- GODEX SUB-AGENT BEGIN -->") != 1 {
		t.Fatalf("sub-agent block duplicated: %s", agents)
	}
}

func TestProdex04356SuperSubAgentOverlayUsesNonemptyOverrideAndProtectsBusySlot(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "AGENTS.md"), []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "AGENTS.override.md"), []byte("override\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := SuperSubAgentConfig{
		Provider: "openai", MaxConcurrency: 4, MaxConcurrencySource: "default",
	}
	if err := PrepareSuperSubAgentOverlay(home, config); err != nil {
		t.Fatal(err)
	}
	base, _ := os.ReadFile(filepath.Join(home, "AGENTS.md"))
	override, _ := os.ReadFile(filepath.Join(home, "AGENTS.override.md"))
	if strings.Contains(string(base), "GODEX SUB-AGENT") ||
		!strings.Contains(string(override), "GODEX SUB-AGENT") {
		t.Fatalf("effective AGENTS selection = base:%q override:%q", base, override)
	}

	slot := filepath.Join(home, "sub-agent-slots", "slot-03.lock")
	release, err := lockfile.TryAcquireExisting(slot)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConcurrency = 3
	config.MaxConcurrencySource = "custom"
	err = PrepareSuperSubAgentOverlay(home, config)
	if err == nil || !strings.Contains(err.Error(), "child holds slot 3") {
		_ = release()
		t.Fatalf("busy stale slot = %v", err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	if err := PrepareSuperSubAgentOverlay(home, config); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(slot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale slot survived reduction: %v", err)
	}
}

func numberedSubAgentRules(value string) int {
	count := 0
	for _, line := range strings.Split(value, "\n") {
		for index := 1; index <= 18; index++ {
			if strings.HasPrefix(line, string(rune('0'+index/10))+string(rune('0'+index%10))+". ") {
				count++
				break
			}
			if index < 10 && strings.HasPrefix(line, string(rune('0'+index))+". ") {
				count++
				break
			}
		}
	}
	return count
}
