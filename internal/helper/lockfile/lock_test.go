package lockfile

import (
	"context"
	"errors"
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
