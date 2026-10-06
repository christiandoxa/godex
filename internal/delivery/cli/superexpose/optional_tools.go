package superexpose

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	runtimecli "github.com/christiandoxa/godex/internal/delivery/cli/runtime"
	presidiogateway "github.com/christiandoxa/godex/internal/gateway/presidio"
)

type optionalTool struct {
	id        string
	kind      string
	available bool
	path      string
	version   string
	detail    string
}

type optionalToolSnapshot struct {
	tools   []optionalTool
	program string
}

var semanticVersionPattern = regexp.MustCompile("[0-9]+[.][0-9]+[.][0-9]+(?:-[0-9A-Za-z.-]+)?")

func discoverOptionalTools() optionalToolSnapshot {
	program, _ := os.Executable()
	if resolved, err := filepath.EvalSymlinks(program); err == nil {
		program = resolved
	}
	result := optionalToolSnapshot{tools: make([]optionalTool, 0, 6), program: program}
	for _, spec := range []struct {
		id   string
		kind string
	}{
		{"caveman", "CodexPlugin"},
		{"rtk", "Command"},
		{"codebase-memory-mcp", "McpServer"},
		{"playwright-mcp", "McpServer"},
		{"ponytail", "CodexPlugin"},
		{"presidio", "Service"},
	} {
		if spec.id == "presidio" {
			result.tools = append(result.tools, discoverExposePresidio())
			continue
		}
		tool := optionalTool{id: spec.id, kind: spec.kind}
		path, ok := runtimecli.ResolveSuperOptionalTool(spec.id)
		if !ok {
			tool.detail = "not found or incompatible"
			result.tools = append(result.tools, tool)
			continue
		}
		tool.available = true
		tool.path = path
		tool.version = exposeResolvedToolVersion(spec.id, path)
		tool.detail = "installed and validated for Godex Super launch"
		result.tools = append(result.tools, tool)
	}
	return result
}

func exposeResolvedToolVersion(id, path string) string {
	switch id {
	case "caveman", "ponytail":
		if version := filepath.Base(path); semanticVersionPattern.MatchString(version) {
			return version
		}
	case "rtk", "codebase-memory-mcp":
		if version, err := probeOptionalToolVersion(path); err == nil {
			return version
		}
	case "playwright-mcp":
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		output, err := exec.CommandContext(ctx, path, "--no-install", "@playwright/mcp", "--version").CombinedOutput()
		if err == nil && ctx.Err() == nil {
			if match := semanticVersionPattern.FindString(string(output)); match != "" {
				return match
			}
		}
	}
	return ""
}

func discoverExposePresidio() optionalTool {
	tool := optionalTool{id: "presidio", kind: "Service"}
	root := strings.TrimSpace(os.Getenv("GODEX_HOME"))
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			tool.detail = "Presidio health could not be validated"
			return tool
		}
		root = filepath.Join(home, ".godex")
	}
	config, _, err := presidiogateway.LoadConfig(root)
	if err != nil {
		tool.detail = "Presidio health could not be validated"
		return tool
	}
	if config.Timeout < 100*time.Millisecond {
		config.Timeout = 100 * time.Millisecond
	}
	if config.Timeout > time.Second {
		config.Timeout = time.Second
	}
	redactor, err := presidiogateway.NewRedactor(config)
	if err != nil {
		tool.detail = "Presidio health could not be validated"
		return tool
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	analyzer := redactor.Probe(ctx, config.AnalyzerURL)
	anonymizer := redactor.Probe(ctx, config.AnonymizerURL)
	if analyzer.OK && anonymizer.OK {
		tool.available = true
		tool.detail = "Presidio Analyzer and Anonymizer are healthy"
		return tool
	}
	tool.detail = "Presidio Analyzer or Anonymizer is not healthy"
	return tool
}

func probeOptionalToolVersion(path string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	if ctx.Err() != nil {
		return "", errorsNew("version probe timed out")
	}
	if err != nil {
		return "", fmt.Errorf("version probe failed: %s", strings.TrimSpace(string(output)))
	}
	match := semanticVersionPattern.FindString(string(output))
	if match == "" {
		return "", errorsNew("version probe returned no semantic version")
	}
	return match, nil
}

func compareSemver(left, right string) int {
	l := parseSemver(left)
	r := parseSemver(right)
	for index := 0; index < 3; index++ {
		if l.numbers[index] < r.numbers[index] {
			return -1
		}
		if l.numbers[index] > r.numbers[index] {
			return 1
		}
	}
	if l.pre == r.pre {
		return 0
	}
	if l.pre == "" {
		return 1
	}
	if r.pre == "" {
		return -1
	}
	return strings.Compare(l.pre, r.pre)
}

type parsedSemver struct {
	numbers [3]int
	pre     string
}

func parseSemver(value string) parsedSemver {
	value = strings.TrimPrefix(value, "v")
	core, pre, _ := strings.Cut(value, "-")
	parts := strings.Split(core, ".")
	var result parsedSemver
	for index := 0; index < len(parts) && index < 3; index++ {
		result.numbers[index], _ = strconv.Atoi(parts[index])
	}
	result.pre = pre
	return result
}

func (snapshot optionalToolSnapshot) availableIDs() []string {
	result := make([]string, 0, len(snapshot.tools))
	for _, tool := range snapshot.tools {
		if tool.available {
			result = append(result, tool.id)
		}
	}
	return result
}

func (snapshot optionalToolSnapshot) tool(id string) (optionalTool, bool) {
	for _, tool := range snapshot.tools {
		if tool.id == id {
			return tool, true
		}
	}
	return optionalTool{}, false
}

func (snapshot optionalToolSnapshot) resolveProgram(program string, args []string) (string, []string, string, error) {
	alias, ok := strings.CutPrefix(program, "optional:")
	if !ok {
		return program, args, "", nil
	}
	integratedPlaywright := alias == "playwright"
	if integratedPlaywright {
		alias = "playwright-mcp"
	}
	tool, known := snapshot.tool(alias)
	if !known {
		return "", nil, "", fmt.Errorf("unknown optional-tool exec alias: %s", alias)
	}
	if !tool.available {
		return "", nil, "", fmt.Errorf("optional tool %s is unavailable: %s", alias, tool.detail)
	}
	switch alias {
	case "rtk", "codebase-memory-mcp":
		if tool.path == "" {
			return "", nil, "", fmt.Errorf("validated optional tool %s has no executable path", alias)
		}
		return tool.path, args, tool.id, nil
	case "playwright-mcp":
		if !integratedPlaywright {
			if tool.path == "" {
				return "", nil, "", errorsNew("validated Playwright MCP has no npx executable path")
			}
			prefix := []string{"--no-install", "@playwright/mcp"}
			return tool.path, append(prefix, args...), tool.id, nil
		}
		if snapshot.program == "" {
			return "", nil, "", errorsNew("Godex executable is unavailable for integrated Playwright")
		}
		prefix := []string{"super", "--no-sub-agent", "--no-presidio", "--tool", "playwright"}
		return snapshot.program, append(prefix, args...), tool.id, nil
	case "caveman", "ponytail":
		if snapshot.program == "" {
			return "", nil, "", errorsNew("Godex executable is unavailable for plugin activation")
		}
		prefix := []string{"super", "--no-sub-agent", "--no-presidio", "--tool", alias}
		return snapshot.program, append(prefix, args...), tool.id, nil
	case "presidio":
		if snapshot.program == "" {
			return "", nil, "", errorsNew("Godex executable is unavailable for Presidio activation")
		}
		prefix := []string{"super", "--no-sub-agent", "--presidio"}
		return snapshot.program, append(prefix, args...), tool.id, nil
	default:
		return "", nil, "", fmt.Errorf("optional tool %s is unavailable", alias)
	}
}

func (snapshot optionalToolSnapshot) applyEnvironment(environment map[string]string) {
	dirs := make([]string, 0, len(snapshot.tools)+8)
	for _, tool := range snapshot.tools {
		if !tool.available || tool.path == "" {
			continue
		}
		parent := filepath.Dir(tool.path)
		if parent != "" && !containsString(dirs, parent) {
			dirs = append(dirs, parent)
		}
		switch tool.id {
		case "rtk":
			environment["GODEX_EXPOSE_RTK_BIN"] = tool.path
		case "codebase-memory-mcp":
			environment["GODEX_EXPOSE_CODEBASE_MEMORY_BIN"] = tool.path
		case "playwright-mcp":
			environment["GODEX_EXPOSE_PLAYWRIGHT_NPX"] = tool.path
		case "caveman":
			environment["GODEX_EXPOSE_CAVEMAN_ROOT"] = tool.path
		case "ponytail":
			environment["GODEX_EXPOSE_PONYTAIL_ROOT"] = tool.path
		}
	}
	basePath := environment["PATH"]
	if basePath == "" {
		basePath = os.Getenv("PATH")
	}
	for _, directory := range filepath.SplitList(basePath) {
		if directory != "" && !containsString(dirs, directory) {
			dirs = append(dirs, directory)
		}
	}
	environment["PATH"] = strings.Join(dirs, string(os.PathListSeparator))
	environment["GODEX_EXPOSE_OPTIONAL_TOOLS"] = strings.Join(snapshot.availableIDs(), ",")
	if tool, ok := snapshot.tool("presidio"); ok && tool.available {
		environment["GODEX_EXPOSE_PRESIDIO_READY"] = "1"
	}
}

func (snapshot optionalToolSnapshot) instructions() string {
	lines := []string{
		"Optional tools are validated once when this expose endpoint starts. Use only entries marked available; unavailable or incompatible tools are not active.",
		"Use program optional:<name> for validated aliases. Normal program names and paths keep their existing behavior.",
	}
	for _, tool := range snapshot.tools {
		if !tool.available {
			lines = append(lines, fmt.Sprintf("- %s: unavailable.", tool.id))
			continue
		}
		version := ""
		if tool.version != "" {
			version = " (" + tool.version + ")"
		}
		invocation := ""
		switch tool.id {
		case "rtk":
			invocation = "program optional:rtk; use it for noisy shell output"
		case "codebase-memory-mcp":
			invocation = "program optional:codebase-memory-mcp; one-shot structural navigation is available"
		case "playwright-mcp":
			invocation = "program optional:playwright-mcp starts the validated MCP package via npx; program optional:playwright launches noninteractive Godex Super with Playwright enabled"
		case "caveman":
			invocation = "program optional:caveman launches noninteractive Godex Super with the validated Caveman plugin enabled"
		case "ponytail":
			invocation = "program optional:ponytail launches noninteractive Godex Super with the validated Ponytail plugin enabled"
		case "presidio":
			invocation = "program optional:presidio launches noninteractive Godex Super with Presidio enabled"
		}
		lines = append(lines, fmt.Sprintf("- %s%s [%s]: %s.", tool.id, version, tool.kind, invocation))
	}
	return strings.Join(lines, "\n")
}

func (snapshot optionalToolSnapshot) manifest() []map[string]any {
	result := make([]map[string]any, 0, len(snapshot.tools))
	for _, tool := range snapshot.tools {
		version := any(nil)
		if tool.version != "" {
			version = tool.version
		}
		result = append(result, map[string]any{
			"id":        tool.id,
			"kind":      tool.kind,
			"available": tool.available,
			"version":   version,
			"alias":     "optional:" + tool.id,
		})
	}
	return result
}

func containsString(values []string, value string) bool {
	for _, current := range values {
		if current == value {
			return true
		}
	}
	return false
}

type stringError string

func (err stringError) Error() string { return string(err) }
func errorsNew(message string) error  { return stringError(message) }
