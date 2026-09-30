package lockfile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExclusiveLockAndCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "guard")
	release, err := TryAcquire(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := TryAcquire(path); !errors.Is(err, ErrBusy) {
		t.Fatalf("second owner = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Acquire(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel = %v", err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	release, err = TryAcquire(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
}

func TestSharedReadersExcludeMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "guard")
	first, err := TryRead(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first()
	second, err := TryRead(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second()
	if _, err := TryAcquire(path); !errors.Is(err, ErrBusy) {
		t.Fatalf("mutation entered shared home: %v", err)
	}
}

func TestLockRejectsSymbolicLinks(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, nil, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, "guard")
	if err := os.Symlink(target, link); err != nil {
		t.Skip(err)
	}
	if _, err := TryAcquire(link); err == nil {
		t.Fatal("symlink lock accepted")
	}
}
