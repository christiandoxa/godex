package openai

import "encoding/json"

const (
	websocketEventTypePrefixBytes = 1024
	websocketEventTypeStringBytes = 128
)

type websocketEventTypeScanner struct {
	depth              int
	inString           bool
	escaped            bool
	stringDepth        int
	captureString      bool
	captureType        bool
	stringOverflow     bool
	stringValue        []byte
	pendingString      bool
	pendingKeyOverflow bool
	pendingKey         []byte
	expectTypeValue    bool
	kind               string
	done               bool
}

func websocketEventTypePrefix(payload []byte) string {
	var scanner websocketEventTypeScanner
	scanner.write(payload)
	return scanner.kind
}

func (scanner *websocketEventTypeScanner) write(payload []byte) {
	for _, value := range payload {
		if scanner.done {
			return
		}
		if scanner.inString {
			scanner.writeString(value)
			continue
		}
		if scanner.pendingString {
			if jsonWhitespace(value) {
				continue
			}
			scanner.pendingString = false
			if value == ':' {
				var key string
				scanner.expectTypeValue = !scanner.pendingKeyOverflow && decodeWebSocketJSONString(scanner.pendingKey, &key) && key == "type"
				scanner.pendingKey = nil
				scanner.pendingKeyOverflow = false
				continue
			}
			scanner.pendingKey = nil
			scanner.pendingKeyOverflow = false
		}
		if scanner.expectTypeValue {
			if jsonWhitespace(value) {
				continue
			}
			scanner.expectTypeValue = false
			if value == '"' {
				scanner.startString(true)
				continue
			}
			scanner.done = true
			continue
		}
		switch value {
		case '"':
			scanner.startString(false)
		case '{', '[':
			scanner.depth++
		case '}', ']':
			if scanner.depth > 0 {
				scanner.depth--
			} else {
				scanner.done = true
			}
		}
	}
}

func (scanner *websocketEventTypeScanner) startString(captureType bool) {
	scanner.inString = true
	scanner.escaped = false
	scanner.stringDepth = scanner.depth
	scanner.captureString = scanner.depth == 1 || captureType
	scanner.captureType = captureType
	scanner.stringOverflow = false
	scanner.stringValue = scanner.stringValue[:0]
}

func (scanner *websocketEventTypeScanner) writeString(value byte) {
	if value == '"' && !scanner.escaped {
		scanner.inString = false
		if scanner.captureType {
			if !scanner.stringOverflow {
				var kind string
				if decodeWebSocketJSONString(scanner.stringValue, &kind) {
					scanner.kind = kind
				}
			}
			scanner.done = true
		} else if scanner.stringDepth == 1 {
			scanner.pendingString = true
			scanner.pendingKey = append(scanner.pendingKey[:0], scanner.stringValue...)
			scanner.pendingKeyOverflow = scanner.stringOverflow
		}
		scanner.captureString = false
		scanner.captureType = false
		return
	}
	if scanner.captureString {
		if len(scanner.stringValue) < websocketEventTypeStringBytes {
			scanner.stringValue = append(scanner.stringValue, value)
		} else {
			scanner.stringOverflow = true
		}
	}
	if scanner.escaped {
		scanner.escaped = false
	} else if value == '\\' {
		scanner.escaped = true
	}
}

func decodeWebSocketJSONString(raw []byte, value *string) bool {
	encoded := append([]byte{'"'}, raw...)
	encoded = append(encoded, '"')
	return json.Unmarshal(encoded, value) == nil
}

func jsonWhitespace(value byte) bool {
	switch value {
	case ' ', '\t', '\r', '\n':
		return true
	default:
		return false
	}
}
