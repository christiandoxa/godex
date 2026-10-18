package session

import (
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

func TestProdex04382RecentWorkspaceSelection(t *testing.T) {
	root := t.TempDir()
	a, b, c := filepath.Join(root, "a"), filepath.Join(root, "b"), filepath.Join(root, "c")
	candidates := []Workspace{
		{Path: a, ModifiedUnix: 1}, {Path: b, ModifiedUnix: 3},
		{Path: c, ModifiedUnix: 3}, {Path: b, ModifiedUnix: 4},
		{Path: "relative", ModifiedUnix: 9}, {Path: "", ModifiedUnix: 10},
		{Path: a + string(filepath.Separator), ModifiedUnix: 2},
	}
	for _, tc := range []struct {
		limit int
		want  []string
	}{
		{0, nil}, {1, []string{b}}, {2, []string{b, c}},
		{9, []string{b, c, a + string(filepath.Separator), a}},
	} {
		got := RecentWorkspaces(candidates, tc.limit)
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("limit %d: got %q, want %q", tc.limit, got, tc.want)
		}
	}
	if candidates[0].Path != a {
		t.Fatal("selection mutated caller order")
	}
	if got := RecentWorkspaces([]Workspace{{Path: c, ModifiedUnix: 1}, {Path: a, ModifiedUnix: 1}, {Path: b, ModifiedUnix: 1}}, 3); !reflect.DeepEqual(got, []string{c, a, b}) {
		t.Fatalf("mtime ties lost input order: %q", got)
	}
	var large []Workspace
	for i := 0; i < 4097; i++ {
		large = append(large, Workspace{Path: filepath.Join(root, strconv.Itoa(i)), ModifiedUnix: int64(i)})
	}
	if got := RecentWorkspaces(large, 4096); len(got) != 4096 || got[0] != large[4096].Path || got[4095] != large[1].Path {
		t.Fatal("4096 workspace cap or newest-first order changed")
	}
}
