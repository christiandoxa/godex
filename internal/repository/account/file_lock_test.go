package account

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestExclusiveLockAndCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "guard")
	release, err := tryFileLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tryFileLock(path); !errors.Is(err, errFileBusy) {
		t.Fatalf("second owner = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := acquireFileLock(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel = %v", err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	release, err = tryFileLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
}
