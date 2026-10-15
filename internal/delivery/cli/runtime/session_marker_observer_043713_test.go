package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProdex043713ObserverMarkerAvailabilityRejectsInvalidWithoutResourceBackoff(t *testing.T) {
	root := t.TempDir()
	marker := &sessionStartMarker04360{directory: root, path: filepath.Join(root, "session.id")}
	if id, available := marker.observedID043713(); id != "" || !available {
		t.Fatalf("absent unstarted session: id=%q available=%t", id, available)
	}
	for _, value := range []string{"invalid-id", "\n", "00000000-0000-0000-0000-000000000000000000"} {
		if err := os.WriteFile(marker.path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
		if id, available := marker.observedID043713(); id != "" || !available {
			t.Fatalf("invalid marker must not masquerade as EMFILE: payload=%q id=%q available=%t", value, id, available)
		}
	}
	if err := os.Remove(marker.path); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "synthetic-target")
	if err := os.WriteFile(target, []byte("01900000-0000-7000-8000-000000000713"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, marker.path); err == nil {
		if id, available := marker.observedID043713(); id != "" || !available {
			t.Fatalf("symlink marker must fail closed without resource retry: id=%q available=%t", id, available)
		}
	}
}
