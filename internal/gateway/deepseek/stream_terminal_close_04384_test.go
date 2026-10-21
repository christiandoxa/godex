package deepseek

import (
	"io"
	"strings"
	"sync/atomic"
	"testing"
)

type deepSeekTerminalReadProbe04384 struct {
	packets []string
	reads   atomic.Int32
	closed  chan struct{}
}

func (source *deepSeekTerminalReadProbe04384) Read(data []byte) (int, error) {
	index := int(source.reads.Add(1)) - 1
	if index < len(source.packets) {
		return copy(data, source.packets[index]), nil
	}
	return 0, io.EOF
}

func (source *deepSeekTerminalReadProbe04384) Close() error {
	close(source.closed)
	return nil
}

func TestProdex04384DeepSeekSSETerminalStateClosesUpstreamWithoutAnotherRead(t *testing.T) {
	const text = "data: {\"id\":\"chat-terminal\",\"choices\":[{\"delta\":{\"content\":\"terminal-control\"}}]}\n\n"
	const done = "data: [DONE]\n\n"
	const ignored = "data: {\"choices\":[{\"delta\":{\"content\":\"must-not-leak\"}}]}\n\n"
	for _, tc := range []struct {
		name, terminal string
		packets        []string
		reads          int32
	}{
		{"done", "response.completed", []string{text + done + ignored}, 1},
		{"embedded-error", "response.failed", []string{"data: {\"error\":{\"code\":\"terminal-error\"}}\n\n" + ignored}, 1},
		{"invalid-tool", "response.failed", []string{"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0}]}}]}\n\n" + ignored}, 1},
		{"separate-done-packet", "response.completed", []string{text, done}, 2},
		{"premature-eof", "response.failed", []string{text}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := &deepSeekTerminalReadProbe04384{packets: tc.packets, closed: make(chan struct{})}
			translated := deepSeekChatSSE(source)
			body, err := io.ReadAll(translated)
			translated.Close()
			if err != nil {
				t.Fatal(err)
			}
			<-source.closed
			if reads := source.reads.Load(); reads != tc.reads {
				t.Errorf("upstream reads=%d want=%d", reads, tc.reads)
			}
			wire := string(body)
			if strings.Count(wire, "event: "+tc.terminal+"\r\n") != 1 || strings.Contains(wire, "must-not-leak") {
				t.Fatalf("terminal output changed: %s", wire)
			}
		})
	}
}
