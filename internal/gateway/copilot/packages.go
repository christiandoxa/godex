package copilot

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

func (source *Source) discoverKeytarPath() (string, error) {
	suffix := filepath.Join("prebuilds", copilotPlatformLabel(runtime.GOOS, runtime.GOARCH), "keytar.node")
	return source.discoverVersionedPackageFile(suffix, "failed to locate the Copilot CLI keychain helper")
}

func (source *Source) discoverSDKPath() (string, error) {
	return source.discoverVersionedPackageFile(filepath.Join("copilot-sdk", "index.js"), "failed to locate the Copilot CLI SDK")
}

func (source *Source) discoverVersionedPackageFile(suffix, missing string) (string, error) {
	type candidate struct {
		version [3]uint64
		path    string
	}
	var candidates []candidate
	for _, root := range source.packageRoots() {
		entries, err := os.ReadDir(root)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		for _, entry := range entries {
			versionRoot := filepath.Join(root, entry.Name())
			if !regularDirectory(versionRoot) || !regularFileUnder(versionRoot, suffix) {
				continue
			}
			candidates = append(candidates, candidate{version: parseVersion(entry.Name()), path: filepath.Join(versionRoot, suffix)})
		}
	}
	if len(candidates) == 0 {
		return "", errors.New(missing)
	}
	sort.Slice(candidates, func(i, j int) bool { return versionLess(candidates[i].version, candidates[j].version) })
	return candidates[len(candidates)-1].path, nil
}

func (source *Source) packageRoots() []string {
	platform := copilotPlatformLabel(runtime.GOOS, runtime.GOARCH)
	seen := map[string]bool{}
	var roots []string
	appendRoot := func(path string) {
		if strings.TrimSpace(path) == "" {
			return
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return
		}
		absolute = filepath.Clean(absolute)
		if !seen[absolute] {
			seen[absolute] = true
			roots = append(roots, absolute)
		}
	}
	if value := strings.TrimSpace(source.getenv("COPILOT_CACHE_HOME")); value != "" {
		appendRoot(filepath.Join(value, "pkg", platform))
	}
	cache := strings.TrimSpace(source.getenv("XDG_CACHE_HOME"))
	if cache == "" {
		if home, err := source.homeDir(); err == nil {
			cache = filepath.Join(home, ".cache")
		}
	}
	appendRoot(filepath.Join(cache, "copilot", "pkg", platform))
	if value := strings.TrimSpace(source.getenv("COPILOT_HOME")); value != "" {
		appendRoot(filepath.Join(value, "pkg", platform))
	}
	if home, err := source.homeDir(); err == nil {
		appendRoot(filepath.Join(home, ".copilot", "pkg", platform))
	}
	return roots
}

func regularDirectory(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink == 0 && info.IsDir()
}

func regularFileUnder(root, suffix string) bool {
	current := root
	for _, component := range strings.Split(filepath.Clean(suffix), string(filepath.Separator)) {
		if component == "." || component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	info, err := os.Lstat(filepath.Join(root, suffix))
	return err == nil && info.Mode()&os.ModeSymlink == 0 && info.Mode().IsRegular()
}

func parseVersion(value string) [3]uint64 {
	parts := strings.Split(value, ".")
	var result [3]uint64
	for index := 0; index < len(result) && index < len(parts); index++ {
		digits := strings.Builder{}
		for _, current := range parts[index] {
			if current < '0' || current > '9' {
				break
			}
			digits.WriteRune(current)
		}
		result[index], _ = strconv.ParseUint(digits.String(), 10, 64)
	}
	return result
}

func versionLess(left, right [3]uint64) bool {
	for index := range left {
		if left[index] != right[index] {
			return left[index] < right[index]
		}
	}
	return false
}

func copilotPlatformLabel(goos, goarch string) string {
	switch goos + "/" + goarch {
	case "linux/amd64":
		return "linux-x64"
	case "linux/arm64":
		return "linux-arm64"
	case "darwin/amd64":
		return "darwin-x64"
	case "darwin/arm64":
		return "darwin-arm64"
	case "windows/amd64":
		return "win32-x64"
	case "windows/arm64":
		return "win32-arm64"
	default:
		return "linux-x64"
	}
}
