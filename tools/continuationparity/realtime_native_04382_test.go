package main

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

type realtimeWriteBuffer04382 struct {
	bytes.Buffer
	writes int
}

func (buffer *realtimeWriteBuffer04382) Write(data []byte) (int, error) {
	buffer.writes++
	return buffer.Buffer.Write(data)
}

func TestProdex04382RealtimeControlFrameUsesSingleTransportWrite(t *testing.T) {
	var reply bytes.Buffer
	if err := websocketframe.WriteFrame(&reply, 1, []byte("ack"), false); err != nil {
		t.Fatal(err)
	}
	var request realtimeWriteBuffer04382
	if err := realtimeRoundTrip04382(&request, bufio.NewReader(&reply), 1, []byte(realtimeUpdate04382), "ack"); err != nil {
		t.Fatal(err)
	}
	if request.writes != 1 {
		t.Fatal("control frame header and payload were split across transport writes")
	}
}

func TestProdex04382RealtimeSaturationReadsLogsWithoutOpeningLockFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX unreadable-file sentinel; Windows native differential covers the held lock")
	}
	root := t.TempDir()
	logs := filepath.Join(root, "logs")
	if err := os.Mkdir(logs, 0700); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(logs, "a.log.lock")
	if err := os.WriteFile(locked, []byte(""), 0000); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(locked); err == nil {
		t.Skip("this account can read files without read permissions")
	}
	if err := os.WriteFile(filepath.Join(logs, "z.log"), []byte("profile_inflight_saturated"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := waitRealtimeSaturation04382(product{state: root}); err != nil {
		t.Fatal(err)
	}
}
