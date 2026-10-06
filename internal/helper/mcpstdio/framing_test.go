package mcpstdio

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestProdex04355MCPFramingReadsJSONLineAndContentLength(t *testing.T) {
	jsonLine := bufio.NewReader(strings.NewReader("\n{\"jsonrpc\":\"2.0\",\"id\":1}\n"))
	message, framing, err := ReadMessage(jsonLine)
	if err != nil || framing != JSONLine || string(message) != `{"id":1,"jsonrpc":"2.0"}` {
		t.Fatalf("json line = %s / %v / %v", message, framing, err)
	}
	body := `{"jsonrpc":"2.0","id":2}`
	content := "Content-Length: 24\r\nX-Test: yes\r\n\r\n" + body
	message, framing, err = ReadMessage(bufio.NewReader(strings.NewReader(content)))
	if err != nil || framing != ContentLength || string(message) != `{"id":2,"jsonrpc":"2.0"}` {
		t.Fatalf("content length = %s / %v / %v", message, framing, err)
	}
}

func TestProdex04355MCPFramingWritesRequestedTransport(t *testing.T) {
	message := []byte(`{"jsonrpc":"2.0","id":1,"result":{}}`)
	var jsonLine bytes.Buffer
	if err := WriteMessage(&jsonLine, message, JSONLine); err != nil {
		t.Fatal(err)
	}
	if got := jsonLine.String(); got != "{\"id\":1,\"jsonrpc\":\"2.0\",\"result\":{}}\n" {
		t.Fatalf("json line = %q", got)
	}
	var framed bytes.Buffer
	if err := WriteMessage(&framed, message, ContentLength); err != nil {
		t.Fatal(err)
	}
	wantBody := `{"id":1,"jsonrpc":"2.0","result":{}}`
	want := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(wantBody), wantBody)
	if framed.String() != want {
		t.Fatalf("framed = %q, want %q", framed.String(), want)
	}
}

func TestProdex04355MCPFramingRejectsBoundsAndDuplicateLength(t *testing.T) {
	for name, fixture := range map[string]struct{ input, want string }{
		"oversized declared body": {input: "Content-Length: 67108865\r\n\r\n", want: "safe size limit"},
		"duplicate length":        {input: "Content-Length: 2\r\nContent-Length: 2\r\n\r\n{}", want: "duplicate MCP Content-Length"},
		"oversized first header":  {input: strings.Repeat(" ", FirstHeaderLineMaxBytes) + "X\n", want: "first line exceeds safe size limit"},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := ReadMessage(bufio.NewReader(strings.NewReader(fixture.input)))
			if err == nil || !strings.Contains(err.Error(), fixture.want) {
				t.Fatalf("error = %v, want %q", err, fixture.want)
			}
		})
	}
}
