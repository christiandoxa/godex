package kiro

import (
	"encoding/json"
	"errors"
	"strings"
)

func decodeACPLine(line []byte) (acpEnvelope, bool, error) {
	if len(strings.TrimSpace(string(line))) == 0 {
		return acpEnvelope{}, true, nil
	}
	// The pinned Rust ACP parser deserializes a typed envelope with strict
	// Unicode, depth and duplicate-known-field checks before accepting a
	// notification. Go json.Unmarshal alone replaces malformed UTF-8 and
	// lets duplicate `method`/`params` impersonate a session update.
	if !validKiroACPTypedEnvelope04384(line) {
		return acpEnvelope{}, false, errors.New("failed to parse Kiro ACP JSON-RPC line")
	}
	var envelope acpEnvelope
	if err := json.Unmarshal(line, &envelope); err != nil || envelope.JSONRPC != "2.0" {
		return acpEnvelope{}, false, errors.New("failed to parse Kiro ACP JSON-RPC line")
	}
	return envelope, false, nil
}
