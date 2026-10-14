package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestProdex043711GeminiExecutableDifferentialNegativeControls(t *testing.T) {
	if !geminiNegativeControl() {
		t.Fatal("the synthetic Gemini oracle failed to reject a material provider or client mutation")
	}
}

func TestProdex043711GeminiSemanticProjectionKeepsMaterialFields(t *testing.T) {
	left := `{"id":"resp-one","created_at":1,"object":"response","model":"gemini-2.5-pro","output":[],"metadata":{},"usage":{},"status":"failed","error":{"code":"gemini_empty_response"}}`
	right := `{"id":"resp-two","created_at":2,"object":"response","model":"gemini-2.5-pro","output":[],"metadata":{},"usage":{},"status":"failed","error":{"code":"gemini_empty_response"}}`
	a, err := geminiResponseProjection(left)
	if err != nil {
		t.Fatal(err)
	}
	b, err := geminiResponseProjection(right)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := a["id"]; exists {
		t.Fatal("the semantic projection retained a randomized response ID")
	}
	if _, exists := a["created_at"]; exists {
		t.Fatal("the semantic projection retained a wall-clock timestamp")
	}
	if !geminiSemanticEqual043711(a, b) {
		t.Fatalf("identical Gemini responses differing only by random IDs and times must compare equal")
	}
	b["metadata"] = map[string]any{"provider": "tampered"}
	if geminiSemanticEqual043711(a, b) {
		t.Fatal("Gemini semantic comparison ignored metadata mutation")
	}
	if _, err := geminiResponseProjection(`{"object":"response"`); err == nil {
		t.Fatal("truncated Gemini response was accepted")
	}
}

func TestProdex043711GeminiFixtureRejectsWrongProtocolAndFakeSuccess(t *testing.T) {
	invalid := geminiCandidateResult{
		Name: "synthetic",
		Upstream: []geminiCapturedRequest{{
			Method: http.MethodPost,
			Path:   "/v1beta/models/gemini-3.1-pro-preview:generateContent",
			Auth:   true,
		}},
		Client: exchange{Status: 200, Body: `{"object":"response","output":[]}`},
	}
	if err := geminiFixtureInvariant("chat-request", invalid); err == nil ||
		!strings.Contains(err.Error(), "unexpected provider route") {
		t.Fatalf("incorrect Gemini protocol bypassed oracle: %v", err)
	}
}

func TestProdex043711GeminiMultiTurnToolOracleNegativeControls(t *testing.T) {
	if !geminiMultiTurnSentinel043711() {
		t.Fatal("Gemini multi-turn oracle accepted an unpaired, duplicate, reordered or changed tool call")
	}
}
