package runtime

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	superPlaywrightPackageCLI     = "@playwright/mcp"
	superPlaywrightMinimumVersion = "0.0.79"
	superToolProbeTimeout         = 5 * time.Second
)

func defaultSuperToolLookup(tool string) (string, bool) {
	switch tool {
	case "caveman", "ponytail":
		return superManagedPluginDirectory(tool)
	case "playwright-mcp":
		return superPlaywrightCommand()
	case "rtk", "codebase-memory-mcp":
		if path, ok := superManagedCommand(tool); ok {
			return path, true
		}
		path, err := exec.LookPath(tool)
		return path, err == nil
	default:
		return "", false
	}
}

func superManagedPluginDirectory(tool string) (string, bool) {
	for _, root := range superOptimizerRoots() {
		candidate, ok := newestSuperManagedVersion(filepath.Join(root, tool))
		if !ok {
			continue
		}
		switch tool {
		case "caveman":
			if info, err := os.Stat(filepath.Join(candidate, "AGENTS.md")); err == nil && info.Mode().IsRegular() {
				return candidate, true
			}
		case "ponytail":
			if info, err := os.Stat(filepath.Join(candidate, ".codex-plugin", "plugin.json")); err == nil && info.Mode().IsRegular() {
				return candidate, true
			}
		}
	}
	return "", false
}

func superManagedCommand(command string) (string, bool) {
	for _, root := range superOptimizerRoots() {
		candidates := []string{filepath.Join(root, command)}
		if command == "codebase-memory-mcp" {
			checkout := filepath.Join(root, command)
			candidates = append(candidates,
				filepath.Join(checkout, command),
				filepath.Join(checkout, "build", "c", command),
				filepath.Join(checkout, "bin", command),
			)
		}
		for _, candidate := range candidates {
			if path, ok := superExecutableCandidate(candidate); ok {
				return path, true
			}
		}
	}
	return "", false
}

func superExecutableCandidate(path string) (string, bool) {
	candidates := []string{path}
	if filepath.Ext(path) == "" {
		for _, extension := range []string{".exe", ".cmd", ".bat"} {
			candidates = append(candidates, path+extension)
		}
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && info.Mode().IsRegular() {
			return candidate, true
		}
	}
	return "", false
}

func superOptimizerRoots() []string {
	roots := make([]string, 0, 8)
	appendUniqueRoot := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		path = filepath.Clean(path)
		for _, existing := range roots {
			if existing == path {
				return
			}
		}
		roots = append(roots, path)
	}
	for _, key := range []string{"GODEX_OPTIMIZERS_HOME", "PRODEX_OPTIMIZERS_HOME"} {
		appendUniqueRoot(os.Getenv(key))
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
		appendUniqueRoot(filepath.Join(xdg, "godex-optimizers"))
		appendUniqueRoot(filepath.Join(xdg, "prodex-optimizers"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		appendUniqueRoot(filepath.Join(home, ".local", "share", "godex-optimizers"))
		appendUniqueRoot(filepath.Join(home, ".local", "share", "prodex-optimizers"))
	}
	return roots
}

func newestSuperManagedVersion(root string) (string, bool) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", false
	}
	bestVersion := ""
	bestPath := ""
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		version := entry.Name()
		if _, ok := parseSuperStableVersion(version); !ok {
			continue
		}
		if bestVersion == "" || compareSuperVersion(version, bestVersion) > 0 {
			bestVersion = version
			bestPath = filepath.Join(root, version)
		}
	}
	return bestPath, bestPath != ""
}

func parseSuperStableVersion(value string) ([3]uint64, bool) {
	var parsed [3]uint64
	if strings.ContainsAny(value, "-+") {
		return parsed, false
	}
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return parsed, false
	}
	for index, part := range parts {
		number, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return parsed, false
		}
		parsed[index] = number
	}
	return parsed, true
}

func compareSuperVersion(left, right string) int {
	a, aok := parseSuperStableVersion(left)
	b, bok := parseSuperStableVersion(right)
	if !aok || !bok {
		return strings.Compare(left, right)
	}
	for index := range a {
		if a[index] < b[index] {
			return -1
		}
		if a[index] > b[index] {
			return 1
		}
	}
	return 0
}

func superPlaywrightCommand() (string, bool) {
	node, err := exec.LookPath("node")
	if err != nil {
		return "", false
	}
	output, ok := superProbeCommand(node, "--version")
	if !ok {
		return "", false
	}
	version := strings.TrimPrefix(strings.TrimSpace(firstSuperLine(output)), "v")
	parts := strings.Split(version, ".")
	major, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil || major < 18 {
		return "", false
	}
	npx, err := exec.LookPath("npx")
	if err != nil {
		return "", false
	}
	output, ok = superProbeCommand(npx, "--no-install", superPlaywrightPackageCLI, "--version")
	if !ok {
		return "", false
	}
	playwrightVersion := strings.TrimPrefix(strings.TrimSpace(firstSuperLine(output)), "v")
	if compareSuperVersion(playwrightVersion, superPlaywrightMinimumVersion) < 0 {
		return "", false
	}
	return npx, true
}

func superProbeCommand(program string, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), superToolProbeTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, program, args...)
	output, err := command.CombinedOutput()
	if err != nil || ctx.Err() != nil {
		return "", false
	}
	return string(output), true
}

func firstSuperLine(value string) string {
	if line, _, found := strings.Cut(value, "\n"); found {
		return strings.TrimSpace(line)
	}
	return strings.TrimSpace(value)
}
