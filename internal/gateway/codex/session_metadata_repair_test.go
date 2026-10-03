package codex

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestRepairSessionMetadataPrefixReReadsCurrentFileAndCreatesBackup(t *testing.T) {
	id := "01900000-0000-7000-8000-000000000020"
	root := t.TempDir()
	path := filepath.Join(root, "sessions", "2026", "07", "16", "rollout-"+id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	current := "{\"timestamp\":\"2026-07-16T01:00:00Z\",\"type\":\"event\",\"payload\":{\"message\":\"current\"}}\n"
	stale := "{\"type\":\"session_meta\",\"payload\":{\"id\":\"" + id + "\"}}\n"
	if err := os.WriteFile(path, []byte(current), 0o644); err != nil {
		t.Fatal(err)
	}

	repaired, err := repairSessionMetadataPrefix(path, stale)
	if err != nil {
		t.Fatal(err)
	}
	if !repaired {
		t.Fatal("current source was not repaired")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(got)), "\n")
	if len(lines) != 2 || !sessionLineStartsCodexRolloutMetadata(lines[0]) || !strings.Contains(lines[1], "current") {
		t.Fatalf("repaired current file = %#v", lines)
	}
	if strings.Contains(string(got), "\"type\":\"session_meta\",\"payload\":{\"id\"") {
		t.Fatalf("stale caller contents leaked into repair: %s", got)
	}
	backup := path + ".prodex-repair-bak"
	if backupBytes, err := os.ReadFile(backup); err != nil || string(backupBytes) != current {
		t.Fatalf("backup = %q, err=%v", backupBytes, err)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("repaired session mode = %#v err=%v", info, err)
		}
		if info, err := os.Stat(backup); err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("backup mode = %#v err=%v", info, err)
		}
		if info, err := os.Stat(filepath.Join(root, sessionRepairLockFile)); err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("repair lock mode = %#v err=%v", info, err)
		}
	}
}

func TestRepairSessionMetadataPrefixLeavesValidSubagentWithoutBackup(t *testing.T) {
	id := "01900000-0000-7000-8000-000000000001"
	path := filepath.Join(t.TempDir(), "rollout-"+id+".jsonl")
	raw := "{\"timestamp\":\"2026-07-11T11:17:19Z\",\"type\":\"session_meta\",\"payload\":{" +
		"\"session_id\":\"other\",\"id\":\"" + id + "\",\"timestamp\":\"2026-07-11T11:17:19Z\"," +
		"\"cwd\":\"/tmp/workspace\",\"originator\":\"codex-tui\",\"cli_version\":\"0.144.1\"," +
		"\"source\":{\"subagent\":{\"thread_spawn\":{\"parent_thread_id\":\"parent\",\"depth\":1}}}}}\n"
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	repaired, err := repairSessionMetadataPrefix(path, "ignored stale caller")
	if err != nil {
		t.Fatal(err)
	}
	if repaired {
		t.Fatal("valid metadata was rewritten")
	}
	if got, _ := os.ReadFile(path); string(got) != raw {
		t.Fatalf("valid session changed: %q", got)
	}
	if _, err := os.Stat(path + ".prodex-repair-bak"); !os.IsNotExist(err) {
		t.Fatalf("valid session created backup: %v", err)
	}
}

func TestRepairSessionMetadataPrefixPreservesZstdCompression(t *testing.T) {
	id := "01900000-0000-7000-8000-000000000001"
	root := t.TempDir()
	path := filepath.Join(root, "sessions", "2026", "08", "19", "rollout-"+id+".jsonl.zst")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	raw := "{\"type\":\"event\"}\n{\"type\":\"session_meta\",\"payload\":{\"id\":\"" + id + "\"}}\n"
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(3)))
	if err != nil {
		t.Fatal(err)
	}
	compressed := encoder.EncodeAll([]byte(raw), nil)
	encoder.Close()
	if err := os.WriteFile(path, compressed, 0o600); err != nil {
		t.Fatal(err)
	}

	repaired, err := repairSessionMetadataPrefixWithSelector(path, id)
	if err != nil {
		t.Fatal(err)
	}
	if !repaired {
		t.Fatal("compressed metadata was not repaired")
	}
	decoded, found, err := readSessionAttachmentFile(path)
	if err != nil || !found {
		t.Fatalf("compressed repaired read = found:%t err=%v", found, err)
	}
	if first := strings.Split(strings.TrimSpace(decoded), "\n")[0]; !sessionLineStartsCodexRolloutMetadata(first) {
		t.Fatalf("compressed repaired prefix = %q", first)
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(onDisk) == decoded || len(onDisk) < 4 || string(onDisk[:4]) != string([]byte{0x28, 0xb5, 0x2f, 0xfd}) {
		t.Fatalf("repaired zstd file is not compressed: %x", onDisk[:min(8, len(onDisk))])
	}
}

func TestRepairSessionMetadataPrefixRejectsSymlinkSource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior covered on Unix")
	}
	root := t.TempDir()
	id := "01900000-0000-7000-8000-000000000001"
	outside := filepath.Join(root, "outside.jsonl")
	link := filepath.Join(root, "rollout-"+id+".jsonl")
	original := "{\"timestamp\":\"2026-01-01T00:00:00Z\",\"type\":\"event\"}\n"
	if err := os.WriteFile(outside, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, err := repairSessionMetadataPrefix(link, ""); err == nil || !strings.Contains(strings.ToLower(err.Error()), "symlink") {
		t.Fatalf("symlink repair error = %v", err)
	}
	if got, _ := os.ReadFile(outside); string(got) != original {
		t.Fatalf("symlink target changed: %q", got)
	}
}

func TestRepairSessionMetadataPrefixAbortsOnConcurrentMutation(t *testing.T) {
	id := "01900000-0000-7000-8000-000000000001"
	root := t.TempDir()
	path := filepath.Join(root, "sessions", "rollout-"+id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	original := "{\"timestamp\":\"2026-01-01T00:00:00Z\",\"type\":\"event\"}\n"
	replacement := "{\"timestamp\":\"2026-01-01T00:00:01Z\",\"type\":\"event\",\"payload\":{\"message\":\"concurrent\"}}\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := repairSessionMetadataPrefixAfterPrepare(path, id, func() {
		if writeErr := os.WriteFile(path, []byte(replacement), 0o600); writeErr != nil {
			t.Fatalf("concurrent mutation: %v", writeErr)
		}
	})
	if err == nil || !strings.Contains(err.Error(), "session changed during repair") {
		t.Fatalf("concurrent repair error = %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != replacement {
		t.Fatalf("concurrent source overwritten: %q", got)
	}
	if _, statErr := os.Stat(path + ".prodex-repair-bak"); !os.IsNotExist(statErr) {
		t.Fatalf("failed repair retained new backup: %v", statErr)
	}
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".*.prodex-repair-tmp-*"))
	if len(matches) != 0 {
		t.Fatalf("failed repair leaked temporary files: %#v", matches)
	}
}

func TestRepairSessionMetadataPrefixRejectsUnsafeBackupWithoutTouchingTargets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink backup case covered on Unix")
	}
	for _, test := range []struct {
		name    string
		prepare func(t *testing.T, backup, outside string)
	}{
		{
			name: "symlink",
			prepare: func(t *testing.T, backup, outside string) {
				t.Helper()
				if err := os.WriteFile(outside, []byte("outside-safe"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, backup); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "directory",
			prepare: func(t *testing.T, backup, _ string) {
				t.Helper()
				if err := os.Mkdir(backup, 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			id := "01900000-0000-7000-8000-000000000018"
			path := filepath.Join(root, "sessions", "rollout-"+id+".jsonl")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			original := "{\"timestamp\":\"2026-07-16T01:00:00Z\",\"type\":\"event\"}\n"
			if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
				t.Fatal(err)
			}
			backup := sessionRepairBackupPath(path)
			outside := filepath.Join(root, "outside-backup")
			test.prepare(t, backup, outside)

			if _, err := repairSessionMetadataPrefix(path, "stale"); err == nil {
				t.Fatal("unsafe backup unexpectedly accepted")
			}
			if got, _ := os.ReadFile(path); string(got) != original {
				t.Fatalf("source changed after backup rejection: %q", got)
			}
			if test.name == "symlink" {
				if got, _ := os.ReadFile(outside); string(got) != "outside-safe" {
					t.Fatalf("outside backup target changed: %q", got)
				}
			}
		})
	}
}

func TestRepairSessionMetadataPrefixPreservesExistingBackup(t *testing.T) {
	root := t.TempDir()
	id := "01900000-0000-7000-8000-000000000021"
	path := filepath.Join(root, "sessions", "rollout-"+id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{\"type\":\"event\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	backup := sessionRepairBackupPath(path)
	const existing = "first-repair-backup"
	if err := os.WriteFile(backup, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := repairSessionMetadataPrefix(path, "stale")
	if err != nil || !changed {
		t.Fatalf("repair = changed:%t err:%v", changed, err)
	}
	if got, _ := os.ReadFile(backup); string(got) != existing {
		t.Fatalf("existing backup overwritten: %q", got)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(backup); err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("existing backup mode = %#v err=%v", info, err)
		}
	}
}
