package superexpose

import (
	"os"
	"path/filepath"
	"reflect"
	goruntime "runtime"
	"strings"
	"testing"
)

func TestProdex04356ExposeOptionalAliasesMatchTaggedActivationModes(t *testing.T) {
	snapshot := optionalToolSnapshot{
		program: "/godex",
		tools: []optionalTool{
			{id: "caveman", kind: "CodexPlugin", available: true, path: "/plugins/caveman", version: "3.0.0"},
			{id: "rtk", kind: "Command", available: true, path: "/bin/rtk", version: "0.50.0"},
			{id: "codebase-memory-mcp", kind: "McpServer", available: true, path: "/bin/codebase-memory-mcp", version: "0.9.1"},
			{id: "playwright-mcp", kind: "McpServer", available: true, path: "/bin/npx", version: "0.0.83"},
			{id: "ponytail", kind: "CodexPlugin", available: true, path: "/plugins/ponytail", version: "4.13.0"},
			{id: "presidio", kind: "Service", available: true, detail: "healthy"},
		},
	}
	cases := []struct {
		program     string
		args        []string
		wantProgram string
		wantArgs    []string
		wantTool    string
	}{
		{"optional:rtk", []string{"gain"}, "/bin/rtk", []string{"gain"}, "rtk"},
		{"optional:codebase-memory-mcp", []string{"cli", "--json"}, "/bin/codebase-memory-mcp", []string{"cli", "--json"}, "codebase-memory-mcp"},
		{"optional:playwright-mcp", []string{"--headless"}, "/bin/npx", []string{"--no-install", "@playwright/mcp", "--headless"}, "playwright-mcp"},
		{"optional:playwright", []string{"exec", "task"}, "/godex", []string{"super", "--no-sub-agent", "--no-presidio", "--tool", "playwright", "exec", "task"}, "playwright-mcp"},
		{"optional:caveman", []string{"exec", "task"}, "/godex", []string{"super", "--no-sub-agent", "--no-presidio", "--tool", "caveman", "exec", "task"}, "caveman"},
		{"optional:ponytail", []string{"exec", "task"}, "/godex", []string{"super", "--no-sub-agent", "--no-presidio", "--tool", "ponytail", "exec", "task"}, "ponytail"},
		{"optional:presidio", []string{"exec", "task"}, "/godex", []string{"super", "--no-sub-agent", "--presidio", "exec", "task"}, "presidio"},
	}
	for _, testCase := range cases {
		t.Run(testCase.program, func(t *testing.T) {
			program, args, tool, err := snapshot.resolveProgram(testCase.program, testCase.args)
			if err != nil {
				t.Fatal(err)
			}
			if program != testCase.wantProgram || tool != testCase.wantTool || !reflect.DeepEqual(args, testCase.wantArgs) {
				t.Fatalf("resolved = program:%q args:%#v tool:%q", program, args, tool)
			}
		})
	}
}

func TestProdex04356ExposeOptionalEnvironmentUsesGodexCanonicalNames(t *testing.T) {
	snapshot := optionalToolSnapshot{
		tools: []optionalTool{
			{id: "caveman", available: true, path: "/plugins/caveman"},
			{id: "rtk", available: true, path: "/tools/rtk"},
			{id: "codebase-memory-mcp", available: true, path: "/memory/codebase-memory-mcp"},
			{id: "playwright-mcp", available: true, path: "/node/npx"},
			{id: "ponytail", available: true, path: "/plugins/ponytail"},
			{id: "presidio", available: true},
		},
	}
	environment := map[string]string{"PATH": "/usr/bin"}
	snapshot.applyEnvironment(environment)
	want := map[string]string{
		"GODEX_EXPOSE_RTK_BIN":             "/tools/rtk",
		"GODEX_EXPOSE_CODEBASE_MEMORY_BIN": "/memory/codebase-memory-mcp",
		"GODEX_EXPOSE_PLAYWRIGHT_NPX":      "/node/npx",
		"GODEX_EXPOSE_CAVEMAN_ROOT":        "/plugins/caveman",
		"GODEX_EXPOSE_PONYTAIL_ROOT":       "/plugins/ponytail",
		"GODEX_EXPOSE_PRESIDIO_READY":      "1",
		"GODEX_EXPOSE_OPTIONAL_TOOLS":      "caveman,rtk,codebase-memory-mcp,playwright-mcp,ponytail,presidio",
	}
	for key, value := range want {
		if environment[key] != value {
			t.Fatalf("%s = %q, want %q", key, environment[key], value)
		}
	}
	for key := range environment {
		if strings.HasPrefix(key, "PRODEX_EXPOSE_") {
			t.Fatalf("legacy Prodex env leaked from canonical expose environment: %s", key)
		}
	}
}

func TestProdex04356ExposeDiscoveryUsesSharedSuperMinimumVersionPolicy(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	root := t.TempDir()
	t.Setenv("GODEX_OPTIMIZERS_HOME", root)
	t.Setenv("PRODEX_OPTIMIZERS_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GODEX_HOME", t.TempDir())

	caveman := filepath.Join(root, "caveman", "2.3.0")
	if err := os.MkdirAll(caveman, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(caveman, "AGENTS.md"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}

	rtk := filepath.Join(root, "rtk")
	if err := os.WriteFile(rtk, []byte("#!/bin/sh\nprintf 'rtk 0.45.9\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	snapshot := discoverOptionalTools()
	if tool, _ := snapshot.tool("caveman"); tool.available {
		t.Fatalf("old Caveman was exposed: %#v", tool)
	}
	if tool, _ := snapshot.tool("rtk"); tool.available {
		t.Fatalf("old RTK was exposed: %#v", tool)
	}

	if err := os.MkdirAll(filepath.Join(root, "caveman", "2.3.1"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "caveman", "2.3.1", "AGENTS.md"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rtk, []byte("#!/bin/sh\nprintf 'rtk 0.46.0\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	snapshot = discoverOptionalTools()
	if tool, _ := snapshot.tool("caveman"); !tool.available || tool.version != "2.3.1" {
		t.Fatalf("minimum Caveman was not exposed: %#v", tool)
	}
	if tool, _ := snapshot.tool("rtk"); !tool.available || tool.version != "0.46.0" {
		t.Fatalf("minimum RTK was not exposed: %#v", tool)
	}
}
