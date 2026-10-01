package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

func TestLogAppendTailAndRotate(t *testing.T) {
	log := NewLog(t.TempDir())
	for index := 0; index < 5; index++ {
		event := runtimemodel.Event{Kind: "request_completed", RequestID: string(rune('a' + index))}
		if err := log.Append(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
	events, err := log.Tail(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[0].RequestID != "c" || events[2].RequestID != "e" {
		t.Fatalf("events = %#v", events)
	}
	if err := os.WriteFile(log.path(), []byte(strings.Repeat("x", maxLogBytes)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := log.Append(context.Background(), runtimemodel.Event{Kind: "request_started"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(log.rotatedPath()); err != nil {
		t.Fatal(err)
	}
}

func TestLogRejectsNonRegularRuntimeFile(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "runtime.jsonl"), 0o700); err != nil {
		t.Fatal(err)
	}
	log := NewLog(root)
	if err := log.Append(context.Background(), runtimemodel.Event{Kind: "request_started"}); err == nil {
		t.Fatal("directory runtime log unexpectedly accepted")
	}
}
