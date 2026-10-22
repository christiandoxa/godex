package main

import (
	"strings"
	"testing"
)

func TestGeminiScenarioSelectionRejectsUnknownBeforeLaunch(t *testing.T) {
	err := runGeminiDifferential("", "", "", "", "missing-scenario")
	if err == nil || !strings.Contains(err.Error(), "unknown Gemini scenario") {
		t.Fatalf("unknown scenario did not fail before launch: %v", err)
	}
}
