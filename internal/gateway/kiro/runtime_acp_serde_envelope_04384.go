package kiro

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/christiandoxa/godex/internal/helper/jsonutil"
)

// Tagged Prodex 0.438.4 deserializes ACP JSON-RPC directly into a typed
// Rust RuntimeKiroAcpEnvelope. Serde rejects repeated recognized struct
// fields, while unknown keys and nested serde_json::Value map fields may
// legitimately repeat (the last nested map value wins). Go's struct
// unmarshaller otherwise silently adopts the final recognized field.
func validKiroACPTypedEnvelope04384(payload []byte) bool {
	if jsonutil.CheckSerdeBoundary(string(payload)) != nil || !jsonutil.CheckSerdeNumberTokens(payload) {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return false
	}
	var recognized [6]bool
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return false
		}
		name, ok := key.(string)
		if !ok {
			return false
		}
		index := -1
		switch name {
		case "jsonrpc":
			index = 0
		case "id":
			index = 1
		case "method":
			index = 2
		case "params":
			index = 3
		case "result":
			index = 4
		case "error":
			index = 5
		}
		if index >= 0 {
			if recognized[index] {
				return false
			}
			recognized[index] = true
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return false
		}
		if name == "error" && string(value) != "null" && !kiroACPErrorFieldsUnique04384(value) {
			return false
		}
	}
	last, err := decoder.Token()
	if err != nil || last != json.Delim('}') {
		return false
	}
	// The typed Rust deserialize also rejects trailing nonwhitespace.
	var trailing any
	return decoder.Decode(&trailing) == io.EOF
}

// RuntimeKiroAcpError is a *typed* serde struct (code, message, data).
// Reject its duplicate known fields while retaining serde Value semantics
// for unknown extensions, even when those extension keys repeat.
func kiroACPErrorFieldsUnique04384(payload []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return false
	}
	var seen [3]bool
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return false
		}
		name, ok := key.(string)
		if !ok {
			return false
		}
		index := -1
		switch name {
		case "code":
			index = 0
		case "message":
			index = 1
		case "data":
			index = 2
		}
		if index >= 0 {
			if seen[index] {
				return false
			}
			seen[index] = true
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return false
		}
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') {
		return false
	}
	// Rust RuntimeKiroAcpError uses required, non-Option fields. Go
	// normally accepts missing or null code/message as their zero values.
	var typed struct {
		Code    *int64  `json:"code"`
		Message *string `json:"message"`
	}
	return json.Unmarshal(payload, &typed) == nil && typed.Code != nil && typed.Message != nil
}
