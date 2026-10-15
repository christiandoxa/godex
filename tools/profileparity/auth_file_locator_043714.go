package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Prodex keeps imported managed profiles in profiles/<name>, while Godex
// stores imported account credentials in accounts/<opaque-id>/codex. Resolve
// only a regular private file inside the fixture's isolated product root;
// never rewrite the external CODEX_HOME or infer one product's private layout
// for the other. Ambiguity fails closed.
func startupManagedAuthFile043714(configHome string) (string, error) {
	var candidates []string
	paths := []string{filepath.Join(configHome, "profiles", "main", "auth.json")}
	accounts, err := filepath.Glob(filepath.Join(configHome, "accounts", "*", "codex", "auth.json"))
	if err != nil {
		return "", err
	}
	paths = append(paths, accounts...)
	for _, path := range paths {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() < 1 || info.Size() > 2<<20 ||
			(runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
			return "", errors.New("imported fixture auth must be a bounded, private regular file")
		}
		relative, err := filepath.Rel(configHome, path)
		if err != nil || filepath.IsAbs(relative) || relative == ".." || len(relative) >= 3 && relative[:3] == "../" {
			return "", errors.New("managed auth file is outside isolated fixture root")
		}
		// Reject ancestor symlinks as well as the file itself. A relative
		// lexical path can otherwise escape through a managed-home alias.
		ancestor := filepath.Dir(path)
		for {
			info, walkErr := os.Lstat(ancestor)
			if walkErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return "", errors.New("imported fixture auth directory cannot be a symlink")
			}
			if ancestor == configHome {
				break
			}
			next := filepath.Dir(ancestor)
			if next == ancestor {
				return "", errors.New("imported fixture auth escaped isolated root")
			}
			ancestor = next
		}
		candidates = append(candidates, path)
	}
	if len(candidates) != 1 {
		return "", fmt.Errorf("expected one imported managed fixture auth file, got %d", len(candidates))
	}
	return candidates[0], nil
}
