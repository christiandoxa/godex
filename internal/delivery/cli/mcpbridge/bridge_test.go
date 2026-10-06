package mcpbridge

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/helper/mcpstdio"
)

type notifyingBuffer struct {
	mu      sync.Mutex
	buf     bytes.Buffer
	written chan struct{}
	once    sync.Once
}

func newNotifyingBuffer() *notifyingBuffer { return &notifyingBuffer{written: make(chan struct{})} }
func (buffer *notifyingBuffer) Write(content []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	count, err := buffer.buf.Write(content)
	buffer.once.Do(func() { close(buffer.written) })
	return count, err
}
func (buffer *notifyingBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buf.String()
}

func TestProdex04355MCPBridgePreservesParentFramingAndUsesChildJSONL(t *testing.T) {
	for _, framing := range []mcpstdio.Framing{mcpstdio.ContentLength, mcpstdio.JSONLine} {
		t.Run(fmt.Sprint(framing), func(t *testing.T) {
			inputReader, inputWriter := io.Pipe()
			output := newNotifyingBuffer()
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			result := make(chan error, 1)
			go func() {
				result <- runWithEnv(ctx, []string{"GO_WANT_HELPER_PROCESS=1"}, os.Args[0], []string{"-test.run=^TestMCPBridgeHelperProcess$"}, inputReader, output)
			}()
			message := []byte(`{"jsonrpc":"2.0","id":7,"method":"tools/list"}`)
			writer := bufio.NewWriter(inputWriter)
			if err := mcpstdio.WriteMessage(writer, message, framing); err != nil {
				t.Fatal(err)
			}
			select {
			case <-output.written:
			case <-ctx.Done():
				t.Fatal("bridge produced no output")
			}
			if err := inputWriter.Close(); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-result:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("bridge did not exit after input EOF")
			}
			reader := bufio.NewReader(strings.NewReader(output.String()))
			response, gotFraming, err := mcpstdio.ReadMessage(reader)
			if err != nil || gotFraming != framing || !strings.Contains(string(response), `"id":7`) || !strings.Contains(string(response), `"result"`) {
				t.Fatalf("bridge response = %s framing=%v err=%v raw=%q", response, gotFraming, err, output.String())
			}
		})
	}
}

func TestProdex04355MCPBridgeReportsBoundedChildStderr(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	inputReader, inputWriter := io.Pipe()
	defer inputWriter.Close()
	err := runWithEnv(ctx, []string{"GO_WANT_FAILURE_HELPER_PROCESS=1"}, os.Args[0], []string{"-test.run=^TestMCPBridgeFailureHelperProcess$"}, inputReader, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "synthetic MCP failure") {
		t.Fatalf("failure error = %v", err)
	}
	if len(err.Error()) > stderrLimit+1024 {
		t.Fatalf("failure diagnostic exceeded bound: %d", len(err.Error()))
	}
}

func TestProdex04355MCPBridgeCleansDescendantHeldPipes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix process-group behavior")
	}
	inputReader, inputWriter := io.Pipe()
	defer inputWriter.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	started := time.Now()
	err := Run(ctx, "sh", []string{"-c", "sleep 30 & exit 0"}, inputReader, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(started) >= 2*time.Second {
		t.Fatalf("descendant-held stdout cleanup took %s", time.Since(started))
	}
}

func TestMCPBridgeHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	reader := bufio.NewReader(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)
	for {
		message, framing, err := mcpstdio.ReadMessage(reader)
		if err != nil {
			os.Exit(2)
		}
		if message == nil {
			return
		}
		if framing != mcpstdio.JSONLine {
			os.Exit(3)
		}
		response := []byte(`{"jsonrpc":"2.0","id":7,"result":{"ok":true}}`)
		if err := mcpstdio.WriteMessage(writer, response, mcpstdio.JSONLine); err != nil {
			os.Exit(4)
		}
		os.Exit(0)
	}
}

func TestMCPBridgeFailureHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_FAILURE_HELPER_PROCESS") != "1" {
		return
	}
	_, _ = io.WriteString(os.Stderr, "synthetic MCP failure\n"+strings.Repeat("x", stderrLimit+1024))
	os.Exit(7)
}
