package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/fileutil"
	"github.com/pelletier/go-toml/v2"
)

const (
	superRTKAwarenessFile       = "RTK.md"
	superOptimizerAwarenessFile = "SUPER_OPTIMIZERS.md"
	superPlaywrightPackage      = "@playwright/mcp"
)

const superRTKAwareness = "# RTK - Rust Token <redacted> (Codex CLI)\n\n" +
	"RTK is a token-optimized CLI proxy for shell commands.\n\n" +
	"Use visible `rtk <cmd>` for noisy terminal work when RTK is installed. If it is unavailable,\n" +
	"report that accurately and run the underlying command normally.\n"

const superOptimizerAwareness = "# Godex Optional Tools\n\n" +
	"Godex resolved optional tools for this temporary launch overlay.\n\n" +
	"- Use visible `rtk <cmd>` for noisy shell output when RTK is available.\n" +
	"- When Codebase Memory MCP is available, use it first for architecture, call-chain, impact,\n" +
	"  and structural code search; run `index_repository` first when the workspace is not indexed.\n" +
	"- Use Playwright MCP for browser work when available.\n" +
	"- Follow Ponytail when its plugin is active.\n" +
	"- Treat Presidio as enabled only when the session status says so.\n\n" +
	"Missing tools are not active. Do not claim they were used.\n"

type SuperOptionalTool struct {
	Name     string
	Path     string
	Required bool
}

func PrepareSuperToolsOverlay(home string, tools []SuperOptionalTool, presidioEnabled bool) error {
	home, err := validateRuntimeHome(home)
	if err != nil {
		return err
	}
	resolved := make(map[string]SuperOptionalTool, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" || name == "presidio" {
			continue
		}
		if strings.TrimSpace(tool.Path) == "" {
			if tool.Required {
				return fmt.Errorf("required optional tool %s has no resolved path", name)
			}
			continue
		}
		info, err := os.Stat(tool.Path)
		if err != nil {
			if tool.Required {
				return fmt.Errorf("required optional tool %s is unavailable: %w", name, err)
			}
			continue
		}
		if name == "caveman" || name == "ponytail" {
			if !info.IsDir() {
				if tool.Required {
					return fmt.Errorf("required optional tool %s did not resolve to a plugin directory", name)
				}
				continue
			}
		} else if !info.Mode().IsRegular() {
			if tool.Required {
				return fmt.Errorf("required optional tool %s did not resolve to an executable file", name)
			}
			continue
		}
		resolved[name] = SuperOptionalTool{Name: name, Path: tool.Path, Required: tool.Required}
	}

	if tool, ok := resolved["caveman"]; ok {
		if err := activateSuperCaveman(home, tool); err != nil {
			return err
		}
	}
	if tool, ok := resolved["rtk"]; ok {
		if err := activateSuperRTK(home, tool.Path); err != nil {
			return err
		}
	}
	if tool, ok := resolved["ponytail"]; ok {
		if err := activateSuperPonytail(home, tool); err != nil {
			return err
		}
	}
	if err := configureSuperMCPServers(home, resolved); err != nil {
		return err
	}
	if err := writeSuperOptimizerAwareness(home, resolved, presidioEnabled); err != nil {
		return err
	}
	return nil
}

func activateSuperCaveman(home string, tool SuperOptionalTool) error {
	agents := filepath.Join(tool.Path, "AGENTS.md")
	info, err := os.Stat(agents)
	if err != nil || !info.Mode().IsRegular() {
		if tool.Required {
			if err == nil {
				err = errors.New("AGENTS.md is not a regular file")
			}
			return fmt.Errorf("required Caveman activation failed: %w", err)
		}
		return nil
	}
	return ensureSuperAgentsReference(home, agents)
}

func activateSuperRTK(home, command string) error {
	path := filepath.Join(home, superRTKAwarenessFile)
	if _, err := fileutil.AtomicWrite(path, []byte(superRTKAwareness)); err != nil {
		return fmt.Errorf("write RTK awareness: %w", err)
	}
	if err := ensureSuperAgentsReference(home, path); err != nil {
		return err
	}
	return configureSuperRTKWrappers(home, command)
}

func writeSuperOptimizerAwareness(home string, tools map[string]SuperOptionalTool, presidioEnabled bool) error {
	available := func(name string) string {
		tool, ok := tools[name]
		if !ok {
			return "no"
		}
		return "yes (" + tool.Path + ")"
	}
	if len(tools) == 0 {
		return nil
	}
	text := superOptimizerAwareness +
		"\n## Available Now\n\n" +
		"- rtk: " + available("rtk") + "\n" +
		"- codebase-memory-mcp: " + available("codebase-memory-mcp") + "\n" +
		"- playwright-mcp: " + available("playwright-mcp") + "\n" +
		"- ponytail plugin: " + available("ponytail") + "\n" +
		"- presidio: " + map[bool]string{true: "enabled", false: "disabled"}[presidioEnabled] + "\n"
	path := filepath.Join(home, superOptimizerAwarenessFile)
	if _, err := fileutil.AtomicWrite(path, []byte(text)); err != nil {
		return fmt.Errorf("write Super optimizer awareness: %w", err)
	}
	return ensureSuperAgentsReference(home, path)
}

func ensureSuperAgentsReference(home, referencePath string) error {
	path, err := effectiveSuperAgentsPath(home)
	if err != nil {
		return err
	}
	content, err := readSuperAgentsFile(path)
	if err != nil {
		return err
	}
	reference := "@" + referencePath
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	retained := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == reference {
			continue
		}
		retained = append(retained, line)
	}
	cleaned := strings.TrimRight(strings.Join(retained, "\n"), "\n")
	var updated string
	if strings.TrimSpace(cleaned) == "" {
		updated = reference + "\n"
	} else {
		updated = cleaned + "\n\n" + reference + "\n"
	}
	if content == updated {
		return nil
	}
	_, err = fileutil.AtomicWrite(path, []byte(updated))
	return err
}

func configureSuperMCPServers(home string, tools map[string]SuperOptionalTool) error {
	codebase, codebaseOK := tools["codebase-memory-mcp"]
	playwright, playwrightOK := tools["playwright-mcp"]
	if !codebaseOK && !playwrightOK {
		return nil
	}
	configPath := filepath.Join(home, "config.toml")
	document, err := readSuperToolConfig(configPath)
	if err != nil {
		return err
	}
	servers, err := ensureSuperToolTable(document, "mcp_servers")
	if err != nil {
		return err
	}
	if codebaseOK {
		executable, err := os.Executable()
		if err != nil {
			return fmt.Errorf("resolve Godex MCP bridge executable: %w", err)
		}
		if resolved, resolveErr := filepath.EvalSymlinks(executable); resolveErr == nil {
			executable = resolved
		}
		expected := map[string]any{
			"command":             executable,
			"args":                []string{"__mcp-jsonl-bridge", codebase.Path},
			"enabled":             true,
			"startup_timeout_sec": int64(60),
		}
		if err := configureSuperMCPServer(servers, "codebase-memory-mcp", expected, codebase.Required); err != nil {
			return err
		}
	}
	if playwrightOK {
		expected := map[string]any{
			"command":                     playwright.Path,
			"args":                        []string{"--no-install", superPlaywrightPackage, "--headless", "--isolated"},
			"enabled":                     true,
			"startup_timeout_sec":         int64(60),
			"default_tools_approval_mode": "writes",
		}
		if err := configureSuperMCPServer(servers, "playwright", expected, playwright.Required); err != nil {
			return err
		}
	}
	rendered, err := toml.Marshal(document)
	if err != nil {
		return fmt.Errorf("render Super optional-tool config: %w", err)
	}
	_, err = fileutil.AtomicWrite(configPath, rendered)
	return err
}

func readSuperToolConfig(path string) (map[string]any, error) {
	content, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return make(map[string]any), nil
	case err != nil:
		return nil, err
	}
	if len(content) > superOverlayTextReadLimit {
		return nil, fmt.Errorf("%s exceeds the %d-byte limit", path, superOverlayTextReadLimit)
	}
	if strings.TrimSpace(string(content)) == "" {
		return make(map[string]any), nil
	}
	var document map[string]any
	if err := toml.Unmarshal(content, &document); err != nil {
		return nil, fmt.Errorf("parse Super optional-tool config: %w", err)
	}
	if document == nil {
		document = make(map[string]any)
	}
	return document, nil
}

func ensureSuperToolTable(parent map[string]any, key string) (map[string]any, error) {
	if value, ok := parent[key]; ok {
		table, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s must be a TOML table", key)
		}
		return table, nil
	}
	table := make(map[string]any)
	parent[key] = table
	return table, nil
}

func configureSuperMCPServer(servers map[string]any, name string, expected map[string]any, required bool) error {
	if value, exists := servers[name]; exists {
		table, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("mcp_servers.%s must be a TOML table", name)
		}
		if required {
			if enabled, ok := table["enabled"]; ok {
				value, ok := enabled.(bool)
				if !ok {
					return fmt.Errorf("mcp_servers.%s enabled must be a boolean", name)
				}
				if !value {
					return fmt.Errorf("required MCP server mcp_servers.%s is disabled; refusing to override user configuration", name)
				}
			}
			for _, key := range []string{"command", "args"} {
				if value, ok := table[key]; ok && !superToolConfigEqual(value, expected[key]) {
					return fmt.Errorf("required MCP server mcp_servers.%s has custom %s; refusing to override user configuration", name, key)
				}
			}
			if _, ok := table["command"]; !ok {
				return fmt.Errorf("required MCP server mcp_servers.%s has incomplete custom configuration", name)
			}
		}
		return nil
	}
	servers[name] = expected
	return nil
}

func superToolConfigEqual(actual, expected any) bool {
	normalize := func(value any) any {
		switch typed := value.(type) {
		case []any:
			result := make([]string, 0, len(typed))
			for _, item := range typed {
				text, ok := item.(string)
				if !ok {
					return value
				}
				result = append(result, text)
			}
			return result
		default:
			return value
		}
	}
	return reflect.DeepEqual(normalize(actual), normalize(expected))
}

func activateSuperPonytail(home string, tool SuperOptionalTool) error {
	pluginJSON := filepath.Join(tool.Path, ".codex-plugin", "plugin.json")
	content, err := os.ReadFile(pluginJSON)
	if err != nil {
		if tool.Required {
			return fmt.Errorf("required Ponytail plugin metadata is unavailable: %w", err)
		}
		return nil
	}
	var metadata struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(content, &metadata); err != nil {
		if tool.Required {
			return fmt.Errorf("required Ponytail plugin metadata is invalid: %w", err)
		}
		return nil
	}
	version := strings.TrimSpace(metadata.Version)
	if version == "" {
		version = "local"
	}
	marketplace := filepath.Join(home, ".tmp", "marketplaces", "ponytail")
	cache := filepath.Join(home, "plugins", "cache", "ponytail", "ponytail", version)
	for _, destination := range []string{
		filepath.Join(marketplace, "plugins", "ponytail"),
		cache,
	} {
		if err := removeSuperOverlayDirectoryPath(destination); err != nil {
			return err
		}
		if err := copySuperToolDirectory(tool.Path, destination); err != nil {
			return err
		}
	}
	manifest := map[string]any{
		"name":      "ponytail",
		"interface": map[string]any{"displayName": "Ponytail"},
		"plugins": []any{map[string]any{
			"name":     "ponytail",
			"source":   map[string]any{"source": "local", "path": "./plugins/ponytail"},
			"policy":   map[string]any{"installation": "AVAILABLE", "authentication": "ON_INSTALL"},
			"category": "Productivity",
		}},
	}
	rendered, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	manifestPath := filepath.Join(marketplace, ".agents", "plugins", "marketplace.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o700); err != nil {
		return err
	}
	if _, err := fileutil.AtomicWrite(manifestPath, append(rendered, '\n')); err != nil {
		return err
	}
	return configureSuperPonytailConfig(home, marketplace)
}

func configureSuperPonytailConfig(home, marketplace string) error {
	path := filepath.Join(home, "config.toml")
	document, err := readSuperToolConfig(path)
	if err != nil {
		return err
	}
	for _, key := range []string{"features", "marketplaces", "plugins"} {
		if value, ok := document[key]; ok {
			if _, ok := value.(map[string]any); !ok {
				return fmt.Errorf("configuration entry %s must be a TOML table", key)
			}
		}
	}
	features, _ := ensureSuperToolTable(document, "features")
	features["plugins"] = true
	features["remote_plugin"] = false
	marketplaces, _ := ensureSuperToolTable(document, "marketplaces")
	pony, err := ensureSuperToolTable(marketplaces, "ponytail")
	if err != nil {
		return err
	}
	pony["source_type"] = "local"
	pony["source"] = marketplace
	delete(pony, "version")
	plugins, _ := ensureSuperToolTable(document, "plugins")
	plugin, err := ensureSuperToolTable(plugins, "ponytail@ponytail")
	if err != nil {
		return err
	}
	plugin["enabled"] = true
	rendered, err := toml.Marshal(document)
	if err != nil {
		return err
	}
	_, err = fileutil.AtomicWrite(path, rendered)
	return err
}

func copySuperToolDirectory(source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must be a real directory", source)
	}
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == ".git" {
			continue
		}
		sourcePath := filepath.Join(source, entry.Name())
		destinationPath := filepath.Join(destination, entry.Name())
		entryInfo, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsupported symbolic link in optional tool %s", sourcePath)
		}
		switch {
		case entryInfo.IsDir():
			if err := copySuperToolDirectory(sourcePath, destinationPath); err != nil {
				return err
			}
		case entryInfo.Mode().IsRegular():
			if err := copySuperOverlayFile(sourcePath, destinationPath); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported file type in optional tool %s", sourcePath)
		}
	}
	return nil
}
