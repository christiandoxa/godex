package runtime

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	superCavemanMinimumVersion    = "2.3.1"
	superRTKMinimumVersion        = "0.46.0"
	superCodebaseMinimumVersion   = "0.9.1-rc.1"
	superPlaywrightPackageCLI     = "@playwright/mcp"
	superPlaywrightMinimumVersion = "0.0.79"
	superPonytailMinimumVersion   = "4.9.0"
	superToolProbeTimeout         = 5 * time.Second
)

func defaultSuperToolLookup(tool string) (string, bool) {
	switch tool {
	case "caveman", "ponytail":
		path, ok := superManagedPluginDirectory(tool)
		if !ok {
			return "", false
		}
		minimum := superCavemanMinimumVersion
		if tool == "ponytail" {
			minimum = superPonytailMinimumVersion
		}
		if compareSuperVersion(filepath.Base(path), minimum) < 0 {
			return "", false
		}
		return path, true
	case "playwright-mcp":
		return superPlaywrightCommand()
	case "rtk", "codebase-memory-mcp":
		path, ok := superManagedCommand(tool)
		if !ok {
			var err error
			path, err = exec.LookPath(tool)
			if err != nil {
				return "", false
			}
		}
		if !superCommandVersionCompatible(tool, path) {
			return "", false
		}
		return path, true
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

type superSemver struct {
	major, minor, patch uint64
	pre                 []string
}

func parseSuperSemver(value string) (superSemver, bool) {
	value = strings.TrimSpace(strings.TrimPrefix(value, "v"))
	core, pre, _ := strings.Cut(value, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return superSemver{}, false
	}
	numbers := [3]uint64{}
	for index, part := range parts {
		number, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return superSemver{}, false
		}
		numbers[index] = number
	}
	result := superSemver{major: numbers[0], minor: numbers[1], patch: numbers[2]}
	if pre != "" {
		result.pre = strings.Split(pre, ".")
	}
	return result, true
}

func compareSuperSemver(left, right string) int {
	a, aok := parseSuperSemver(left)
	b, bok := parseSuperSemver(right)
	if !aok || !bok {
		return strings.Compare(left, right)
	}
	for _, pair := range [][2]uint64{{a.major, b.major}, {a.minor, b.minor}, {a.patch, b.patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	if len(a.pre) == 0 && len(b.pre) == 0 {
		return 0
	}
	if len(a.pre) == 0 {
		return 1
	}
	if len(b.pre) == 0 {
		return -1
	}
	limit := min(len(a.pre), len(b.pre))
	for index := 0; index < limit; index++ {
		leftID, rightID := a.pre[index], b.pre[index]
		leftNum, leftErr := strconv.ParseUint(leftID, 10, 64)
		rightNum, rightErr := strconv.ParseUint(rightID, 10, 64)
		switch {
		case leftErr == nil && rightErr == nil:
			if leftNum < rightNum {
				return -1
			}
			if leftNum > rightNum {
				return 1
			}
		case leftErr == nil:
			return -1
		case rightErr == nil:
			return 1
		default:
			if leftID < rightID {
				return -1
			}
			if leftID > rightID {
				return 1
			}
		}
	}
	if len(a.pre) < len(b.pre) {
		return -1
	}
	if len(a.pre) > len(b.pre) {
		return 1
	}
	return 0
}

func superProbeSemver(value string) (string, bool) {
	for _, token := range strings.Fields(value) {
		token = strings.TrimFunc(token, func(r rune) bool {
			return !((r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') ||
				(r >= 'a' && r <= 'z') || r == '.' || r == '-' || r == '+')
		})
		token = strings.TrimPrefix(token, "v")
		if _, ok := parseSuperSemver(token); ok {
			return token, true
		}
	}
	return "", false
}

func superCommandVersionCompatible(tool, path string) bool {
	output, ok := superProbeCommand(path, "--version")
	if !ok {
		return false
	}
	line := firstSuperLine(output)
	switch tool {
	case "rtk":
		version, ok := superProbeSemver(line)
		return ok && compareSuperSemver(version, superRTKMinimumVersion) >= 0
	case "codebase-memory-mcp":
		const prefix = "codebase-memory-mcp "
		if !strings.HasPrefix(line, prefix) {
			return false
		}
		version := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		if version == "dev" {
			return true
		}
		_, ok := parseSuperSemver(version)
		return ok && compareSuperSemver(version, superCodebaseMinimumVersion) >= 0
	default:
		return false
	}
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
	if compareSuperSemver(playwrightVersion, superPlaywrightMinimumVersion) < 0 {
		return "", false
	}
	return npx, true
}

func firstSuperLine(value string) string {
	if line, _, found := strings.Cut(value, "\n"); found {
		return strings.TrimSpace(line)
	}
	return strings.TrimSpace(value)
}

// ResolveSuperOptionalTool exposes the same validated optional-tool resolution used by Godex Super.
// The returned path is either an executable path or a managed plugin directory depending on the tool kind.
func ResolveSuperOptionalTool(tool string) (string, bool) {
	return defaultSuperToolLookup(tool)
}
