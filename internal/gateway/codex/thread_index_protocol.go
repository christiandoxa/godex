package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	threadIndexPageLimit     = 100
	threadIndexClientName    = "prodex-thread-index-reconciliation"
	threadIndexClientVersion = "0.435.6"
	threadIndexMaxJSONBytes  = 64 * 1024 * 1024
	threadIndexMaxJSONNodes  = 1_048_576
)

var (
	errThreadIndexJSONBytes = errors.New("codex app-server thread-index JSON input exceeded its ABI bound")
	errThreadIndexJSONNodes = errors.New("codex app-server thread-index JSON tree exceeded its ABI bound")
)

func reconcileCodexThreadIndexProtocol(stdout io.Reader, stdin io.Writer) error {
	reader := bufio.NewReader(stdout)
	requestID := uint64(1)
	if err := writeCodexAppServerMessage(stdin, map[string]any{
		"id": requestID, "method": "initialize",
		"params": map[string]any{"clientInfo": map[string]string{
			"name": threadIndexClientName, "version": threadIndexClientVersion,
		}},
	}); err != nil {
		return err
	}
	if _, err := readCodexAppServerResponse(reader, requestID); err != nil {
		return err
	}
	if err := writeCodexAppServerMessage(stdin, map[string]any{"method": "initialized"}); err != nil {
		return err
	}

	for _, archived := range []bool{false, true} {
		var cursor *string
		seenCursors := make(map[string]struct{})
		for {
			requestID++
			if err := writeCodexAppServerMessage(stdin, map[string]any{
				"id": requestID, "method": "thread/list",
				"params": map[string]any{
					"archived": archived, "cursor": cursor, "limit": threadIndexPageLimit,
					"modelProviders": []string{}, "sortKey": "updated_at", "sourceKinds": []string{},
					"useStateDbOnly": false,
				},
			}); err != nil {
				return err
			}
			result, err := readCodexAppServerResponse(reader, requestID)
			if err != nil {
				return err
			}
			nextCursor, err := codexThreadListCursor(result)
			if err != nil {
				return err
			}
			if nextCursor == nil {
				break
			}
			if _, exists := seenCursors[*nextCursor]; exists {
				return errors.New("codex app-server repeated a thread list cursor")
			}
			seenCursors[*nextCursor] = struct{}{}
			cursor = nextCursor
		}
	}
	return nil
}

func writeCodexAppServerMessage(writer io.Writer, message any) error {
	if err := json.NewEncoder(writer).Encode(message); err != nil {
		return fmt.Errorf("write codex app-server request: %w", err)
	}
	return nil
}

func readCodexAppServerResponse(reader *bufio.Reader, requestID uint64) (json.RawMessage, error) {
	for {
		line, readErr := readBoundedThreadIndexLine(reader)
		if errors.Is(readErr, errThreadIndexJSONBytes) {
			return nil, readErr
		}
		if len(line) == 0 {
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					return nil, errors.New("codex app-server stopped during thread index repair")
				}
				return nil, fmt.Errorf("read codex app-server response: %w", readErr)
			}
			continue
		}
		if err := validateThreadIndexJSON(line); err != nil {
			if errors.Is(err, errThreadIndexJSONNodes) {
				return nil, err
			}
			return nil, errors.New("codex app-server returned invalid JSON during thread index repair")
		}
		var message map[string]json.RawMessage
		if err := json.Unmarshal(line, &message); err != nil {
			return nil, errors.New("codex app-server returned invalid JSON during thread index repair")
		}
		var id uint64
		idRaw, hasID := message["id"]
		if !hasID || json.Unmarshal(idRaw, &id) != nil || id != requestID {
			if readErr != nil && !errors.Is(readErr, io.EOF) {
				return nil, fmt.Errorf("read codex app-server response: %w", readErr)
			}
			continue
		}
		if errorRaw, exists := message["error"]; exists && string(errorRaw) != "null" {
			detail := "unknown app-server error"
			var appError map[string]any
			if json.Unmarshal(errorRaw, &appError) == nil {
				if message, ok := appError["message"].(string); ok && message != "" {
					detail = message
				}
			}
			return nil, fmt.Errorf("Codex thread index reconciliation failed: %s", detail)
		}
		result, exists := message["result"]
		if !exists {
			return nil, errors.New("codex app-server response is missing its result")
		}
		return result, nil
	}
}

func readBoundedThreadIndexLine(reader *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		chunk, err := reader.ReadSlice('\n')
		if len(line) > threadIndexMaxJSONBytes || len(chunk) > threadIndexMaxJSONBytes-len(line) {
			return nil, errThreadIndexJSONBytes
		}
		needed := len(line) + len(chunk)
		if needed > cap(line) {
			capacity := cap(line) * 2
			if capacity < needed {
				capacity = needed
			}
			if capacity > threadIndexMaxJSONBytes {
				capacity = threadIndexMaxJSONBytes
			}
			grown := make([]byte, len(line), capacity)
			copy(grown, line)
			line = grown
		}
		line = append(line, chunk...)
		if err == nil || !errors.Is(err, bufio.ErrBufferFull) {
			return line, err
		}
	}
}

type threadIndexJSONFrame struct {
	kind       byte
	expectsKey bool
}

func validateThreadIndexJSON(content []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	frames := make([]threadIndexJSONFrame, 0, 8)
	nodes := 0
	tooManyNodes := false
	rootSeen := false
	rootDone := false

	beginValue := func() bool {
		if rootDone {
			return false
		}
		if len(frames) == 0 {
			if rootSeen {
				return false
			}
			rootSeen = true
		} else if frames[len(frames)-1].kind == '{' && frames[len(frames)-1].expectsKey {
			return false
		}
		if nodes < threadIndexMaxJSONNodes {
			nodes++
		} else {
			tooManyNodes = true
		}
		return true
	}
	completeValue := func() {
		if len(frames) == 0 {
			rootDone = true
			return
		}
		if frames[len(frames)-1].kind == '{' {
			frames[len(frames)-1].expectsKey = true
		}
	}

	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			if !rootDone {
				return errors.New("incomplete JSON")
			}
			if tooManyNodes {
				return errThreadIndexJSONNodes
			}
			return nil
		}
		if err != nil {
			return err
		}
		if delimiter, ok := token.(json.Delim); ok {
			switch delimiter {
			case '{':
				if !beginValue() {
					return errors.New("invalid JSON value")
				}
				frames = append(frames, threadIndexJSONFrame{kind: '{', expectsKey: true})
			case '[':
				if !beginValue() {
					return errors.New("invalid JSON value")
				}
				frames = append(frames, threadIndexJSONFrame{kind: '['})
			case '}':
				if len(frames) == 0 || frames[len(frames)-1].kind != '{' || !frames[len(frames)-1].expectsKey {
					return errors.New("invalid JSON object")
				}
				frames = frames[:len(frames)-1]
				completeValue()
			case ']':
				if len(frames) == 0 || frames[len(frames)-1].kind != '[' {
					return errors.New("invalid JSON array")
				}
				frames = frames[:len(frames)-1]
				completeValue()
			}
			continue
		}
		if len(frames) > 0 && frames[len(frames)-1].kind == '{' && frames[len(frames)-1].expectsKey {
			if _, ok := token.(string); !ok {
				return errors.New("invalid JSON object key")
			}
			frames[len(frames)-1].expectsKey = false
			continue
		}
		if !beginValue() {
			return errors.New("multiple JSON values")
		}
		completeValue()
	}
}

func codexThreadListCursor(result json.RawMessage) (*string, error) {
	var value struct {
		NextCursor json.RawMessage `json:"nextCursor"`
	}
	if err := json.Unmarshal(result, &value); err != nil {
		return nil, errors.New("codex app-server returned an invalid thread list result")
	}
	if len(value.NextCursor) == 0 || string(value.NextCursor) == "null" {
		return nil, nil
	}
	var cursor string
	if err := json.Unmarshal(value.NextCursor, &cursor); err != nil {
		return nil, errors.New("codex app-server returned an invalid thread list cursor")
	}
	return &cursor, nil
}
