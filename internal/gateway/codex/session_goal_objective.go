package codex

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// persistSessionGoalObjectiveAttachments repairs only native-generated goal wrappers. Raw pasted
// text and image files remain opaque, even when their bytes happen to contain an attachment path.
func persistSessionGoalObjectiveAttachments(codexHome, objective string) error {
	root := filepath.Join(codexHome, sessionAttachmentDir)
	for _, rawPath := range sessionPersistedAttachmentPaths(objective) {
		path := decodeSessionPath(rawPath)
		relative, ok := sessionAttachmentPathSuffix(path)
		if !ok || filepath.Base(path) != "goal-objective.md" {
			continue
		}
		wrapper := filepath.Join(root, relative)
		safe, err := sessionGeneratedGoalPathSafe(root, wrapper)
		if err != nil {
			return err
		}
		if !safe {
			continue
		}
		contents, found, err := readSessionAttachmentFile(wrapper)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		rewritten, err := rewriteSessionPersistedAttachmentPaths(codexHome, contents)
		if err != nil {
			return err
		}
		if rewritten == contents {
			continue
		}
		// A user or native process may have replaced the wrapper while it was being read.
		current, found, err := readSessionAttachmentFile(wrapper)
		if err != nil {
			return err
		}
		if !found || current != contents {
			continue
		}
		if err := writeSessionAttachmentFile(wrapper, rewritten); err != nil {
			return fmt.Errorf("repair generated goal attachment %s: %w", wrapper, err)
		}
	}
	return nil
}

func sessionGeneratedGoalPathSafe(root, path string) (bool, error) {
	if !sessionPathStartsWith(path, root) || filepath.Base(path) != "goal-objective.md" {
		return false, nil
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect generated goal attachment: %w", err)
	}
	if !info.Mode().IsRegular() {
		return false, nil
	}
	relative, err := filepath.Rel(root, filepath.Dir(path))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return false, nil
	}
	current := root
	if relative != "." {
		for _, component := range strings.Split(relative, string(filepath.Separator)) {
			if component == "" || component == "." {
				continue
			}
			current = filepath.Join(current, component)
			parent, err := os.Lstat(current)
			if errors.Is(err, os.ErrNotExist) {
				return false, nil
			}
			if err != nil {
				return false, fmt.Errorf("inspect generated goal attachment directory: %w", err)
			}
			if parent.Mode()&os.ModeSymlink != 0 || !parent.IsDir() {
				return false, nil
			}
		}
	}
	return true, nil
}
