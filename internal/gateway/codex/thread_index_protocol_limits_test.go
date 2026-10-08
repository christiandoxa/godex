package codex

import (
	"bufio"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestReadBoundedThreadIndexLineAcceptsJustUnderLimit(t *testing.T) {
	line, err := readBoundedThreadIndexLine(bufio.NewReader(newThreadIndexSizedJSONReader(threadIndexMaxJSONBytes - 1)))
	if err != nil {
		t.Fatal(err)
	}
	if len(line) != threadIndexMaxJSONBytes-1 {
		t.Fatalf("line length = %d, want %d", len(line), threadIndexMaxJSONBytes-1)
	}
}

func TestReadBoundedThreadIndexLineRejectsOverLimit(t *testing.T) {
	_, err := readBoundedThreadIndexLine(bufio.NewReader(newThreadIndexSizedJSONReader(threadIndexMaxJSONBytes + 1)))
	if !errors.Is(err, errThreadIndexJSONBytes) {
		t.Fatalf("error = %v, want byte-limit error", err)
	}
}

func TestValidateThreadIndexJSONEnforcesNodeLimit(t *testing.T) {
	for _, test := range []struct {
		name     string
		elements int
		wantErr  error
	}{
		{name: "just under", elements: threadIndexMaxJSONNodes - 2},
		{name: "over", elements: threadIndexMaxJSONNodes, wantErr: errThreadIndexJSONNodes},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateThreadIndexJSON(threadIndexNullArray(test.elements))
			if test.wantErr == nil {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want node-limit error", err)
			}
		})
	}
}

func TestThreadIndexProtocolRejectsMalformedJSON(t *testing.T) {
	var written strings.Builder
	err := reconcileCodexThreadIndexProtocol(strings.NewReader("{\n"), &written)
	if err == nil || !strings.Contains(err.Error(), "returned invalid JSON") {
		t.Fatalf("error = %v", err)
	}
}

func threadIndexNullArray(elements int) []byte {
	var content strings.Builder
	content.Grow(elements*5 + 2)
	content.WriteByte('[')
	for index := 0; index < elements; index++ {
		if index > 0 {
			content.WriteByte(',')
		}
		content.WriteString("null")
	}
	content.WriteByte(']')
	return []byte(content.String())
}

type threadIndexSizedJSONReader struct {
	prefix     []byte
	suffix     []byte
	repeatLeft int
	phase      int
}

func newThreadIndexSizedJSONReader(size int) io.Reader {
	prefix := []byte(`{"id":1,"result":{"nextCursor":null},"padding":"`)
	suffix := []byte(`"}` + "\n")
	return &threadIndexSizedJSONReader{
		prefix:     prefix,
		suffix:     suffix,
		repeatLeft: size - len(prefix) - len(suffix),
	}
}

func (reader *threadIndexSizedJSONReader) Read(target []byte) (int, error) {
	for len(target) > 0 {
		switch reader.phase {
		case 0:
			if len(reader.prefix) == 0 {
				reader.phase = 1
				continue
			}
			n := copy(target, reader.prefix)
			reader.prefix = reader.prefix[n:]
			return n, nil
		case 1:
			if reader.repeatLeft == 0 {
				reader.phase = 2
				continue
			}
			n := min(len(target), reader.repeatLeft)
			for index := 0; index < n; index++ {
				target[index] = 'x'
			}
			reader.repeatLeft -= n
			return n, nil
		case 2:
			if len(reader.suffix) == 0 {
				reader.phase = 3
				continue
			}
			n := copy(target, reader.suffix)
			reader.suffix = reader.suffix[n:]
			return n, nil
		default:
			return 0, io.EOF
		}
	}
	return 0, nil
}
