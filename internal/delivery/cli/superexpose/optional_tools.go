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
	tools []optionalTool
}

var semanticVersionPattern = regexp.MustCompile("[0-9]+[.][0-9]+[.][0-9]+(?:-[0-9A-Za-z.-]+)?")

func discoverOptionalTools() optionalToolSnapshot {
	specs := []struct {
		id      string
		kind    string
		command string
		minimum string
	}{
		{"caveman", "CodexPlugin", "", "2.3.1"},
		{"rtk", "Command", "rtk", "0.46.0"},
		{"codebase-memory-mcp", "McpServer", "codebase-memory-mcp", "0.9.1-rc.1"},
		{"playwright-mcp", "McpServer", "playwright-mcp", "0.0.79"},
		{"ponytail", "CodexPlugin", "", "4.9.0"},
		{"presidio", "Service", "", ""},
	}
	result := optionalToolSnapshot{tools: make([]optionalTool, 0, len(specs))}
	for _, spec := range specs {
		tool := optionalTool{id: spec.id, kind: spec.kind}
		if spec.command == "" {
			tool.detail = "integration is unavailable in this Godex build"
			result.tools = append(result.tools, tool)
			continue
		}
		path, err := exec.LookPath(spec.command)
		if err != nil {
			tool.detail = "not found"
			result.tools = append(result.tools, tool)
			continue
		}
		version, err := probeOptionalToolVersion(path)
		if err != nil {
			tool.path = path
			tool.detail = err.Error()
			result.tools = append(result.tools, tool)
			continue
		}
		if compareSemver(version, spec.minimum) < 0 {
			tool.path = path
			tool.version = version
			tool.detail = fmt.Sprintf("version %s is older than minimum %s", version, spec.minimum)
			result.tools = append(result.tools, tool)
			continue
		}
		tool.available = true
		tool.path = path
		tool.version = version
		tool.detail = "installed and validated for Godex Super launch"
		result.tools = append(result.tools, tool)
	}
	return result
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
	if alias == "playwright" {
		return "", nil, "", fmt.Errorf("optional tool playwright is unavailable: integrated Playwright activation is not implemented yet")
	}
	tool, known := snapshot.tool(alias)
	if !known {
		return "", nil, "", fmt.Errorf("unknown optional-tool exec alias: %s", alias)
	}
	if !tool.available {
		return "", nil, "", fmt.Errorf("optional tool %s is unavailable: %s", alias, tool.detail)
	}
	if tool.path == "" {
		return "", nil, "", fmt.Errorf("validated optional tool %s has no executable path", alias)
	}
	return tool.path, args, tool.id, nil
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
			environment["GODEX_EXPOSE_PLAYWRIGHT_BIN"] = tool.path
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
		lines = append(lines, fmt.Sprintf("- %s%s [%s]: available.", tool.id, version, tool.kind))
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
