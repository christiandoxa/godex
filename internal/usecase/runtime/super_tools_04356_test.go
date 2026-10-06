package runtime

import (
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestProdex04356SuperToolsOverlayActivatesRTKAndMCPServers(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("model = \"base\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "AGENTS.md"), []byte("base instructions\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rtk := writeSuperToolFixture(t, "rtk")
	codebase := writeSuperToolFixture(t, "codebase-memory-mcp")
	npx := writeSuperToolFixture(t, "npx")

	if err := PrepareSuperToolsOverlay(home, []SuperOptionalTool{
		{Name: "rtk", Path: rtk},
		{Name: "codebase-memory-mcp", Path: codebase, Required: true},
		{Name: "playwright-mcp", Path: npx, Required: true},
	}, true); err != nil {
		t.Fatal(err)
	}

	config := readSuperToolsTestConfig(t, filepath.Join(home, "config.toml"))
	servers, ok := config["mcp_servers"].(map[string]any)
	if !ok {
		t.Fatalf("mcp_servers = %#v", config["mcp_servers"])
	}
	codebaseServer, ok := servers["codebase-memory-mcp"].(map[string]any)
	if !ok {
		t.Fatalf("codebase-memory server = %#v", servers["codebase-memory-mcp"])
	}
	command, _ := codebaseServer["command"].(string)
	if !filepath.IsAbs(command) || !strings.Contains(command, "godex") && !strings.Contains(command, ".test") {
		t.Fatalf("codebase bridge command = %q", command)
	}
	if got := stringSliceFromTOML(codebaseServer["args"]); len(got) != 2 ||
		got[0] != "__mcp-jsonl-bridge" || got[1] != codebase {
		t.Fatalf("codebase args = %#v", got)
	}
	if codebaseServer["enabled"] != true {
		t.Fatalf("codebase enabled = %#v", codebaseServer["enabled"])
	}

	playwright, ok := servers["playwright"].(map[string]any)
	if !ok {
		t.Fatalf("playwright server = %#v", servers["playwright"])
	}
	if playwright["command"] != npx {
		t.Fatalf("playwright command = %#v", playwright["command"])
	}
	if got := stringSliceFromTOML(playwright["args"]); strings.Join(got, ",") != "--no-install,@playwright/mcp,--headless,--isolated" {
		t.Fatalf("playwright args = %#v", got)
	}
	if playwright["enabled"] != true || playwright["startup_timeout_sec"] != int64(60) ||
		playwright["default_tools_approval_mode"] != "writes" {
		t.Fatalf("playwright fields = %#v", playwright)
	}

	for _, path := range []string{
		filepath.Join(home, "RTK.md"),
		filepath.Join(home, "SUPER_OPTIMIZERS.md"),
		superRTKWrapperPath(home),
	} {
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("activation artifact %s = %v / %v", path, info, err)
		}
	}
	agents, err := os.ReadFile(filepath.Join(home, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, reference := range []string{"@" + filepath.Join(home, "RTK.md"), "@" + filepath.Join(home, "SUPER_OPTIMIZERS.md")} {
		if strings.Count(string(agents), reference) != 1 {
			t.Fatalf("AGENTS missing/doubled %q: %s", reference, agents)
		}
	}
	awareness, _ := os.ReadFile(filepath.Join(home, "SUPER_OPTIMIZERS.md"))
	if !strings.Contains(string(awareness), "- presidio: enabled") ||
		!strings.Contains(string(awareness), codebase) ||
		!strings.Contains(string(awareness), npx) {
		t.Fatalf("optimizer awareness = %s", awareness)
	}
}

func TestProdex04356SuperToolsRequiredMCPRefusesCustomConfig(t *testing.T) {
	home := t.TempDir()
	codebase := writeSuperToolFixture(t, "codebase-memory-mcp")
	config := "[mcp_servers.codebase-memory-mcp]\nenabled = true\ncommand = \"custom-codebase\"\nargs = [\"--custom\"]\n"
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	err := PrepareSuperToolsOverlay(home, []SuperOptionalTool{{
		Name: "codebase-memory-mcp", Path: codebase, Required: true,
	}}, false)
	if err == nil || !strings.Contains(err.Error(), "custom command") {
		t.Fatalf("required custom MCP config = %v", err)
	}
	content, readErr := os.ReadFile(filepath.Join(home, "config.toml"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(content) != config {
		t.Fatalf("required conflict mutated config:\n%s", content)
	}
}

func TestProdex04356SuperToolsOptionalMCPPreservesExistingTable(t *testing.T) {
	home := t.TempDir()
	npx := writeSuperToolFixture(t, "npx")
	config := "[mcp_servers.playwright]\nenabled = false\ncommand = \"custom-playwright\"\nargs = [\"--headed\"]\n"
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := PrepareSuperToolsOverlay(home, []SuperOptionalTool{{
		Name: "playwright-mcp", Path: npx,
	}}, false); err != nil {
		t.Fatal(err)
	}
	document := readSuperToolsTestConfig(t, filepath.Join(home, "config.toml"))
	servers := document["mcp_servers"].(map[string]any)
	playwright := servers["playwright"].(map[string]any)
	if playwright["command"] != "custom-playwright" || playwright["enabled"] != false ||
		strings.Join(stringSliceFromTOML(playwright["args"]), ",") != "--headed" {
		t.Fatalf("optional custom config overwritten = %#v", playwright)
	}
}

func TestProdex04356SuperToolsActivatesCavemanAndPonytailDirectories(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.toml"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	caveman := filepath.Join(t.TempDir(), "caveman")
	if err := os.MkdirAll(caveman, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(caveman, "AGENTS.md"), []byte("caveman guidance\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ponytail := filepath.Join(t.TempDir(), "ponytail")
	if err := os.MkdirAll(filepath.Join(ponytail, ".codex-plugin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ponytail, ".codex-plugin", "plugin.json"), []byte(`{"version":"4.13.0"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ponytail, "plugin.txt"), []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := PrepareSuperToolsOverlay(home, []SuperOptionalTool{
		{Name: "caveman", Path: caveman, Required: true},
		{Name: "ponytail", Path: ponytail, Required: true},
	}, false); err != nil {
		t.Fatal(err)
	}
	agents, err := os.ReadFile(filepath.Join(home, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(agents), "@"+filepath.Join(caveman, "AGENTS.md")) {
		t.Fatalf("Caveman reference missing: %s", agents)
	}
	for _, path := range []string{
		filepath.Join(home, ".tmp", "marketplaces", "ponytail", "plugins", "ponytail", "plugin.txt"),
		filepath.Join(home, ".tmp", "marketplaces", "ponytail", ".agents", "plugins", "marketplace.json"),
		filepath.Join(home, "plugins", "cache", "ponytail", "ponytail", "4.13.0", "plugin.txt"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("Ponytail artifact %s: %v", path, err)
		}
	}
	config := readSuperToolsTestConfig(t, filepath.Join(home, "config.toml"))
	features := config["features"].(map[string]any)
	if features["plugins"] != true || features["remote_plugin"] != false {
		t.Fatalf("Ponytail features = %#v", features)
	}
	marketplaces := config["marketplaces"].(map[string]any)
	ponyMarket := marketplaces["ponytail"].(map[string]any)
	if ponyMarket["source_type"] != "local" {
		t.Fatalf("Ponytail marketplace = %#v", ponyMarket)
	}
	plugins := config["plugins"].(map[string]any)
	if plugins["ponytail@ponytail"].(map[string]any)["enabled"] != true {
		t.Fatalf("Ponytail plugin config = %#v", plugins)
	}
}

func writeSuperToolFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("fixture"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func readSuperToolsTestConfig(t *testing.T, path string) map[string]any {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := toml.Unmarshal(content, &document); err != nil {
		t.Fatal(err)
	}
	return document
}

func stringSliceFromTOML(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func superRTKWrapperPath(home string) string {
	if goruntime.GOOS == "windows" {
		return filepath.Join(home, "bin", "rtk.cmd")
	}
	return filepath.Join(home, "bin", "rtk")
}
