package runtime

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestProdex04360CompressedRecoveryCheckpointRejectsStaleAndTamperedHistory(t *testing.T) {
	const session = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	baseline := []byte(`{"type":"session_meta","payload":{"id":"` + session + `"}}` + "\n" +
		`{"type":"response_item","payload":{"role":"user"}}` + "\n" +
		`{"type":"error","error":{"code":"usage_limit_reached"}}` + "\n")
	fresh := []byte(`{"type":"response_item","payload":{"role":"user"}}` + "\n" +
		`{"type":"error","error":{"code":"usage_limit_reached"}}` + "\n")
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()
	for _, tc := range []struct {
		name    string
		change  []byte
		replace bool
		want    bool
	}{
		{"stale compressed history", nil, false, false},
		{"fresh recompressed valid suffix", fresh, true, true},
		{"fresh frame appended", fresh, false, true},
		{"tampered historical prefix", fresh, true, false},
		{"incomplete fresh record", bytes.TrimSuffix(fresh, []byte("\n")), true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "rollout-"+session+".jsonl.zst")
			if err := os.WriteFile(path, encoder.EncodeAll(baseline, nil), 0600); err != nil {
				t.Fatal(err)
			}
			checkpoint := captureRecoveryCheckpoint04360(path)
			if !checkpoint.valid {
				t.Fatal("compressed checkpoint not created")
			}
			if tc.change != nil {
				next := append(append([]byte(nil), baseline...), tc.change...)
				if tc.name == "tampered historical prefix" {
					changed := bytes.Replace(baseline, []byte(session),
						[]byte("1"+session[1:]), 1)
					if len(changed) != len(baseline) {
						t.Fatal("tamper fixture changed decoded prefix length")
					}
					next = append(changed, tc.change...)
				}
				if tc.replace {
					newer := filepath.Join(root, "tmp.zst")
					if err := os.WriteFile(newer, encoder.EncodeAll(next, nil), 0600); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(newer, path); err != nil {
						t.Fatal(err)
					}
				} else {
					f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
					if err != nil {
						t.Fatal(err)
					}
					_, err = f.Write(encoder.EncodeAll(tc.change, nil))
					if err != nil {
						t.Fatal(err)
					}
					if err = f.Close(); err != nil {
						t.Fatal(err)
					}
				}
			}
			got := checkpoint.newAcceptedUsageLimit04360(context.Background(), session)
			if got != tc.want {
				t.Fatalf("compressed checkpoint signal=%t want=%t", got, tc.want)
			}
		})
	}
}

func TestProdex04360CompressedRecoveryRejectsBombAndSymlink(t *testing.T) {
	const session = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	root := t.TempDir()
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()
	baseline := []byte(`{"type":"session_meta","payload":{"id":"` + session + `"}}` + "\n")
	path := filepath.Join(root, "rollout-"+session+".jsonl.zst")
	if err := os.WriteFile(path, encoder.EncodeAll(baseline, nil), 0600); err != nil {
		t.Fatal(err)
	}
	cp := captureRecoveryCheckpoint04360(path)
	if !cp.valid {
		t.Fatal("missing baseline")
	}
	var bomb bytes.Buffer
	bomb.Write(baseline)
	bomb.Write(bytes.Repeat([]byte("x"), int(recoveryScanCap04360+100)))
	bomb.Write([]byte("\n"))
	bomb.Write([]byte(`{"type":"response_item","payload":{"role":"user"}}` + "\n" +
		`{"type":"error","error":{"code":"usage_limit_reached"}}` + "\n"))
	if err := os.WriteFile(path, encoder.EncodeAll(bomb.Bytes(), nil), 0600); err != nil {
		t.Fatal(err)
	}
	if cp.newAcceptedUsageLimit04360(context.Background(), session) {
		t.Fatal("decompression bomb unlocked recovery")
	}
	link := filepath.Join(root, "linked.jsonl.zst")
	if err := os.Symlink(path, link); err == nil && captureRecoveryCheckpoint04360(link).valid {
		t.Fatal("symlink baseline accepted")
	}
}
