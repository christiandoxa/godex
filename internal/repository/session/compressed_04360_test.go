package session

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestProdex04360ReaderDiscoversCompressedRolloutMetadata(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "sessions", "2026", "10", "08")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"type":"session_meta","payload":{"id":"` + threadID + `","source":"exec","cwd":"/repo","model_provider":"openai"}}` + "\n" +
		`{"type":"turn_context","payload":{"model":"gpt-6.1-sol","effort":"ultra"}}` + "\n")
	enc, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	data := enc.EncodeAll(payload, nil)
	enc.Close()
	path := filepath.Join(dir, "rollout-2026-10-08T00-00-00-"+threadID+".jsonl.zst")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	reports, err := NewReader().List(t.Context(), home)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].ID != threadID || reports[0].LastModel != "gpt-6.1-sol" ||
		reports[0].LastReasoningEffort != "ultra" || reports[0].Source != "exec" || reports[0].Path != path {
		t.Fatalf("compressed metadata not discoverable: %+v", reports)
	}
	link := filepath.Join(dir, "rollout-forged.jsonl.zst")
	if err := os.Symlink(path, link); err == nil {
		after, err := NewReader().List(t.Context(), home)
		if err != nil || len(after) != 1 {
			t.Fatalf("symlink compressed metadata followed: %v %+v", err, after)
		}
	}
}

func TestProdex04360ReaderCompressedMetadataOutputBounded(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "sessions")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	initial := []byte(`{"type":"session_meta","payload":{"id":"` + threadID + `"}}` + "\n")
	// A decompression bomb may contain a valid initial record but no
	// large-body read can be allowed past the 4 MiB scanning budget.
	payload := append(initial, bytes.Repeat([]byte("x"), maxSessionScanBytes*2)...)
	enc, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	data := enc.EncodeAll(payload, nil)
	enc.Close()
	path := filepath.Join(dir, "rollout-"+threadID+".jsonl.zst")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	reports, err := NewReader().List(t.Context(), home)
	if err != nil || len(reports) != 1 || reports[0].ID != threadID {
		t.Fatalf("bounded compressed scan lost leading session header: reports=%+v err=%v", reports, err)
	}
}
