package runtime

import (
	"bytes"
	"strings"
	"testing"
)

func TestProdex04357SuperDryRunSubAgentReportMatchesTaggedSurface(t *testing.T) {
	options := superOptions{
		dryRun:   true,
		presidio: true,
		subAgent: superSubAgent{
			enabled: true, provider: "local", model: "gpt-5.6",
			effort: "high", url: "http://127.0.0.1:11434/v1",
			maxConcurrency: 4, maxConcurrencySource: "default",
		},
		requiredTools: []string{"codebase-memory-mcp", "rtk"},
		codexArgs:     []string{"resume", "00000000-0000-7000-8000-000000000042"},
	}
	var out bytes.Buffer
	if err := renderSuperDryRunResolved(&out, options, nil); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{
		"Sub-agent: enabled",
		"Sub-agent provider: Local",
		"Sub-agent model: gpt-5.6",
		"Sub-agent reasoning effort: high",
		"Maximum active sub-agents: 4 (Godex default)",
		"Sub-agent concurrency hard maximum: 64",
		"Sub-agent concurrency enforcement: cross-process exclusive slot leases",
		"Sub-agent inherited Presidio: enabled",
		"Sub-agent inherited required tools: codebase-memory-mcp, rtk",
		"Sub-agent local URL: configured",
		"Sub-agent launch target: resume <SESSION_UUID> (parent resume id is not inherited by children)",
		"Sub-agent recursion disabled: yes",
		"Sub-agent recursion marker: GODEX_SUB_AGENT=1",
		"Sub-agent child launcher: shell-free internal command",
		"Sub-agent overlay: SUB_AGENTS.md (temporary; full instructions injected into the effective AGENTS file)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Super dry-run missing %q: %s", want, text)
		}
	}
	if strings.Contains(text, "http://127.0.0.1:11434/v1") ||
		strings.Contains(text, "00000000-0000-7000-8000-000000000042") {
		t.Fatalf("Super dry-run leaked local URL or parent session UUID: %s", text)
	}
}
