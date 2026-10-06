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

func TestConcurrentFirstAcquireCreatesPersistentLockFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "guard")
	const workers = 16
	type result struct {
		release func() error
		err     error
	}
	start := make(chan struct{})
	results := make(chan result, workers)
	for range workers {
		go func() {
			<-start
			release, err := TryAcquire(path)
			results <- result{release: release, err: err}
		}()
	}
	close(start)
	acquired := 0
	var releases []func() error
	for range workers {
		result := <-results
		if result.err != nil {
			if !errors.Is(result.err, ErrBusy) {
				t.Fatalf("unexpected acquire error: %v", result.err)
			}
			continue
		}
		acquired++
		releases = append(releases, result.release)
	}
	for _, release := range releases {
		if err := release(); err != nil {
			t.Fatal(err)
		}
	}
	if acquired != 1 {
		t.Fatalf("exclusive acquire count = %d, want 1", acquired)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("lock mode = %v", info.Mode())
	}
}

func TestTryAcquireExistingNeverCreatesSlot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "slot-00.lock")
	if _, err := TryAcquireExisting(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing existing slot = %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing slot was created: %v", err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	release, err := TryAcquireExisting(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := TryAcquireExisting(path); !errors.Is(err, ErrBusy) {
		t.Fatalf("contended existing slot = %v", err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
}
