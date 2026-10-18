package session

import (
	"path/filepath"
	"slices"
)

type Workspace struct {
	Path         string
	ModifiedUnix int64
}

func RecentWorkspaces(candidates []Workspace, limit int) []string {
	if limit <= 0 {
		return nil
	}
	ordered := slices.Clone(candidates)
	slices.SortStableFunc(ordered, func(a, b Workspace) int {
		if a.ModifiedUnix > b.ModifiedUnix {
			return -1
		}
		if a.ModifiedUnix < b.ModifiedUnix {
			return 1
		}
		return 0
	})
	var selected []string
	seen := make(map[string]bool)
	for _, candidate := range ordered {
		if !filepath.IsAbs(candidate.Path) || seen[candidate.Path] {
			continue
		}
		seen[candidate.Path] = true
		selected = append(selected, candidate.Path)
		if len(selected) == limit {
			break
		}
	}
	return selected
}
