//go:build linux

package processresource

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

const descriptorHeadroomChildEnv = "GODEX_DESCRIPTOR_HEADROOM_TEST_CHILD"

func TestDescriptorHeadroomRecoversStateReadUnderLowLimit(t *testing.T) {
	if os.Getenv(descriptorHeadroomChildEnv) == "" {
		command := exec.Command(os.Args[0], "-test.run=^TestDescriptorHeadroomRecoversStateReadUnderLowLimit$", "-test.v")
		command.Env = append(os.Environ(), descriptorHeadroomChildEnv+"=1")
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("descriptor-limit child failed: %v\n%s", err, output)
		}
		if !bytes.Contains(output, []byte("DESCRIPTOR_STATE_READ_RECOVERED")) {
			t.Fatalf("descriptor-limit child did not report recovery:\n%s", output)
		}
		return
	}

	var original syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &original); err != nil {
		t.Fatal(err)
	}
	hard := min(original.Max, 256)
	if hard < 128 {
		t.Skipf("descriptor regression needs a hard limit >=128, got %d", hard)
	}

	root := t.TempDir()
	state := filepath.Join(root, "state.json")
	content := []byte(`{"active_profile":"synthetic-owner"}`)
	if err := os.WriteFile(state, content, 0o600); err != nil {
		t.Fatal(err)
	}
	low := syscall.Rlimit{Cur: 64, Max: hard}
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &low); err != nil {
		t.Fatal(err)
	}

	var held []*os.File
	defer func() {
		for _, file := range held {
			_ = file.Close()
		}
	}()
	for {
		file, err := os.Open(os.DevNull)
		if err == nil {
			held = append(held, file)
			continue
		}
		if !errors.Is(err, syscall.EMFILE) {
			t.Fatal(err)
		}
		break
	}
	if _, err := os.ReadFile(state); !errors.Is(err, syscall.EMFILE) {
		t.Fatalf("state read under exhausted descriptors = %v, want EMFILE", err)
	}

	if err := PrepareDescriptorHeadroom(); err != nil {
		t.Fatal(err)
	}
	var after syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &after); err != nil {
		t.Fatal(err)
	}
	if after.Max != hard || after.Cur != hard {
		t.Fatalf("descriptor limits after recovery = cur:%d max:%d, want %d", after.Cur, after.Max, hard)
	}
	got, err := os.ReadFile(state)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("state content after descriptor recovery = %q", got)
	}
	fmt.Fprintln(os.Stdout, "DESCRIPTOR_STATE_READ_RECOVERED")
}
