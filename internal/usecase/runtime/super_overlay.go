package runtime

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

var superOverlaySharedDirectories = []string{
	"sessions",
	"archived_sessions",
	"attachments",
	"image_attachments",
}

const superOverlayTextReadLimit = 512 * 1024

var superOverlayAppCacheDirectories = []string{
	"cache/codex_apps_server_info",
	"cache/codex_apps_tools",
	"cache/codex_app_directory",
}

type SuperOverlay struct {
	Home string
}

func PrepareSuperOverlay(managedRoot, baseHome string) (*SuperOverlay, error) {
	root, err := validateSuperOverlayRoot(managedRoot)
	if err != nil {
		return nil, err
	}
	base, err := validateRuntimeHome(baseHome)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(base)
	if err != nil {
		return nil, fmt.Errorf("inspect base CODEX_HOME: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("base CODEX_HOME must be a real directory")
	}

	overlay, err := os.MkdirTemp(root, ".godex-overlay-")
	if err != nil {
		return nil, fmt.Errorf("allocate Godex Super overlay: %w", err)
	}
	if err := os.Chmod(overlay, 0o700); err != nil {
		_ = os.RemoveAll(overlay)
		return nil, err
	}
	cleanup := func(err error) (*SuperOverlay, error) {
		_ = os.RemoveAll(overlay)
		return nil, err
	}
	if err := copySuperOverlayHome(base, overlay); err != nil {
		return cleanup(err)
	}
	for _, relative := range superOverlayAppCacheDirectories {
		if err := removeSuperOverlayDirectoryPath(filepath.Join(overlay, relative)); err != nil {
			return cleanup(err)
		}
	}
	if err := shareSuperOverlayHistory(base, overlay); err != nil {
		return cleanup(err)
	}
	if err := shareSuperOverlayRolloutState(base, overlay); err != nil {
		return cleanup(err)
	}
	if err := configureSuperOverlay(overlay); err != nil {
		return cleanup(err)
	}
	return &SuperOverlay{Home: overlay}, nil
}

func (overlay *SuperOverlay) Close() error {
	if overlay == nil || strings.TrimSpace(overlay.Home) == "" {
		return nil
	}
	home := overlay.Home
	overlay.Home = ""
	return os.RemoveAll(home)
}

func validateSuperOverlayRoot(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("managed profile root is required")
	}
	if !filepath.IsAbs(path) {
		return "", errors.New("managed profile root must be absolute")
	}
	path = filepath.Clean(path)
	if path == filepath.Dir(path) {
		return "", errors.New("managed profile root must not be filesystem root")
	}
	info, err := os.Lstat(path)
	switch {
	case err == nil:
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("managed profile root %s must not be a symbolic link", path)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("managed profile root %s must be a directory", path)
		}
	case errors.Is(err, os.ErrNotExist):
		if err := os.MkdirAll(path, 0o700); err != nil {
			return "", fmt.Errorf("create managed profile root: %w", err)
		}
	default:
		return "", fmt.Errorf("inspect managed profile root: %w", err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return "", fmt.Errorf("secure managed profile root: %w", err)
	}
	return path, nil
}

func copySuperOverlayHome(source, destination string) error {
	sourceRoot, err := filepath.EvalSymlinks(source)
	if err != nil {
		return fmt.Errorf("resolve base CODEX_HOME: %w", err)
	}
	sourceRoot, err = filepath.Abs(sourceRoot)
	if err != nil {
		return err
	}
	return copySuperOverlayDirectory(sourceRoot, source, destination, true)
}

func copySuperOverlayDirectory(sourceRoot, source, destination string, root bool) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return fmt.Errorf("read CODEX_HOME directory %s: %w", source, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if root && superOverlayRootEntrySkipped(name) {
			continue
		}
		sourcePath := filepath.Join(source, name)
		destinationPath := filepath.Join(destination, name)
		info, err := entry.Info()
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("inspect %s: %w", sourcePath, err)
		}
		switch {
		case entry.Type()&os.ModeSymlink != 0:
			if err := copySuperOverlaySymlinkedFile(sourceRoot, sourcePath, destinationPath); err != nil {
				return err
			}
		case info.IsDir():
			if err := os.MkdirAll(destinationPath, 0o700); err != nil {
				return err
			}
			if err := os.Chmod(destinationPath, 0o700); err != nil {
				return err
			}
			if err := copySuperOverlayDirectory(sourceRoot, sourcePath, destinationPath, false); err != nil {
				return err
			}
		case info.Mode().IsRegular():
			if err := copySuperOverlayFile(sourcePath, destinationPath); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return err
			}
		}
	}
	return nil
}

func superOverlayRootEntrySkipped(name string) bool {
	switch name {
	case "packages", "history.jsonl", "sessions", "archived_sessions", "attachments", "image_attachments":
		return true
	default:
		return false
	}
}

func copySuperOverlaySymlinkedFile(sourceRoot, sourcePath, destinationPath string) error {
	target, err := filepath.EvalSymlinks(sourcePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return err
	}
	if !pathWithinRoot(sourceRoot, target) {
		return nil
	}
	info, err := os.Stat(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a file", target)
	}
	return copySuperOverlayFile(target, destinationPath)
}

func pathWithinRoot(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative != ".." && relative != "." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func copySuperOverlayFile(source, destination string) error {
	before, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !before.Mode().IsRegular() {
		return fmt.Errorf("%s is not a file", source)
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	after, err := input.Stat()
	if err != nil {
		return err
	}
	if !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return fmt.Errorf("source file changed while opening %s", source)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, before.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	syncErr := output.Sync()
	closeErr := output.Close()
	if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
		_ = os.Remove(destination)
		return err
	}
	if err := os.Chmod(destination, before.Mode().Perm()); err != nil {
		_ = os.Remove(destination)
		return err
	}
	if err := os.Chtimes(destination, before.ModTime(), before.ModTime()); err != nil {
		_ = os.Remove(destination)
		return err
	}
	return nil
}

func shareSuperOverlayHistory(base, overlay string) error {
	history := filepath.Join(base, "history.jsonl")
	if _, err := os.Lstat(history); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(history, nil, 0o600); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if err := replaceSuperOverlayWithSymlink(history, filepath.Join(overlay, "history.jsonl"), false); err != nil {
		return err
	}
	for _, name := range superOverlaySharedDirectories {
		source := filepath.Join(base, name)
		if err := os.MkdirAll(source, 0o700); err != nil {
			return err
		}
		if err := replaceSuperOverlayWithSymlink(source, filepath.Join(overlay, name), true); err != nil {
			return err
		}
	}
	return nil
}

func shareSuperOverlayRolloutState(base, overlay string) error {
	entries, err := os.ReadDir(base)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !superOverlayRolloutStateName(entry.Name()) {
			continue
		}
		source := filepath.Join(base, entry.Name())
		if err := replaceSuperOverlayWithSymlink(source, filepath.Join(overlay, entry.Name()), false); err != nil {
			return err
		}
	}
	return nil
}

func superOverlayRolloutStateName(name string) bool {
	if !strings.HasPrefix(name, "state_") {
		return false
	}
	return strings.HasSuffix(name, ".sqlite") ||
		strings.HasSuffix(name, ".sqlite-shm") ||
		strings.HasSuffix(name, ".sqlite-wal")
}

func replaceSuperOverlayWithSymlink(target, link string, directory bool) error {
	if err := removeSuperOverlayPath(link); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		return err
	}
	return createSuperOverlaySymlink(target, link, directory)
}

func removeSuperOverlayDirectoryPath(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return os.Remove(path)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	return os.RemoveAll(path)
}

func removeSuperOverlayPath(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return os.Remove(path)
	}
	return os.RemoveAll(path)
}

const legacyCavemanInstructionsSHA256 = "a07e8d5167e454f7637eb0e35230a84987c537ae98d6f795d246338b652810f1"

func configureSuperOverlay(overlay string) error {
	configPath := filepath.Join(overlay, "config.toml")
	if err := removeLegacyCavemanConfig(configPath); err != nil {
		return err
	}
	for _, relative := range []string{
		".tmp/marketplaces/prodex-caveman",
		"plugins/cache/prodex-caveman",
	} {
		if err := removeSuperOverlayDirectoryPath(filepath.Join(overlay, relative)); err != nil {
			return err
		}
	}
	return nil
}

func removeLegacyCavemanConfig(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > superOverlayTextReadLimit {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	content, readErr := io.ReadAll(io.LimitReader(file, superOverlayTextReadLimit+1))
	closeErr := file.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return err
	}
	if len(content) > superOverlayTextReadLimit {
		return nil
	}
	if strings.TrimSpace(string(content)) == "" {
		return nil
	}
	var document map[string]any
	if err := toml.Unmarshal(content, &document); err != nil {
		return fmt.Errorf("parse overlay config: %w", err)
	}
	changed := removeLegacyCavemanTableEntry(document, "marketplaces", "prodex-caveman")
	changed = removeLegacyCavemanTableEntry(document, "plugins", "caveman@prodex-caveman") || changed
	changed = removeLegacyCavemanInstructions(document) || changed
	if !changed {
		return nil
	}
	rendered, err := toml.Marshal(document)
	if err != nil {
		return fmt.Errorf("render Godex Super overlay config: %w", err)
	}
	return os.WriteFile(path, rendered, 0o600)
}

func removeLegacyCavemanTableEntry(document map[string]any, parent, key string) bool {
	child, ok := document[parent].(map[string]any)
	if !ok {
		return false
	}
	if _, exists := child[key]; !exists {
		return false
	}
	delete(child, key)
	if len(child) == 0 {
		delete(document, parent)
	}
	return true
}

func removeLegacyCavemanInstructions(document map[string]any) bool {
	instructions, ok := document["developer_instructions"].(string)
	if !ok {
		return false
	}
	paragraphs := strings.Split(instructions, "\n\n")
	retained := paragraphs[:0]
	for _, paragraph := range paragraphs {
		digest := fmt.Sprintf("%x", sha256.Sum256([]byte(paragraph)))
		if digest != legacyCavemanInstructionsSHA256 {
			retained = append(retained, paragraph)
		}
	}
	if len(retained) == len(paragraphs) {
		return false
	}
	if len(retained) == 0 {
		delete(document, "developer_instructions")
	} else {
		document["developer_instructions"] = strings.Join(retained, "\n\n")
	}
	return true
}

func superOverlayMode(path string) (fs.FileMode, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return 0, err
	}
	return info.Mode(), nil
}
