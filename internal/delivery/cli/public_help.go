package cli

import (
	"fmt"
	"io"

	runtimecli "github.com/christiandoxa/godex/internal/delivery/cli/runtime"
)

var publicHelpText = map[string]string{
	"profile": `Add, inspect, remove, and activate managed Godex profiles.

Usage: godex profile <COMMAND>

Commands:
  add             Add a profile entry and optionally seed it from another CODEX_HOME
  export          Export one or more profiles, including supported profile secrets
  import          Import profiles from an export bundle or supported built-in source
  import-current  Copy the current shared Codex home into a managed profile and activate it
  list            List configured profiles and show which one is active
  remove          Remove one profile entry or every profile entry and optionally delete managed homes
  use             Set the active profile used by commands that omit --profile

Options:
  -h, --help  Print help
`,
	"profile/add": `Add a profile entry and optionally seed it from another CODEX_HOME.

Usage: godex profile add [OPTIONS] <NAME>

Options:
      --codex-home PATH  Register an existing CODEX_HOME
      --copy-from PATH   Copy initial state from another CODEX_HOME
      --copy-current     Seed from the default shared Codex home
      --activate         Make the profile active after creation
      --insecure         Bypass private-file permission/ACL validation
  -h, --help             Print help
`,
	"profile/export": `Export one or more profiles, including supported profile secrets.

Usage: godex profile export [OPTIONS] [PATH]

Options:
  -p, --profile NAME   Export only this profile; repeat to select multiple profiles
      --password-protect
                       Protect the export bundle with a password
      --no-password    Explicitly export without password protection
  -h, --help           Print help
`,
	"profile/import": `Import profiles from an export bundle or supported built-in source.

Usage: godex profile import [OPTIONS] <PATH_OR_SOURCE>

Options:
      --name NAME      Override the imported profile name for a built-in source
      --activate       Activate a built-in import immediately
      --insecure       Bypass private-file permission/ACL validation
  -h, --help           Print help
`,
	"profile/import-current": `Copy the current shared Codex home into a managed profile and activate it.

Usage: godex profile import-current [OPTIONS] [NAME]

Options:
      --insecure  Bypass private-file permission/ACL validation
  -h, --help      Print help
`,
	"profile/list": `List configured profiles and show which one is active.

Usage: godex profile list

Options:
  -h, --help  Print help
`,
	"profile/remove": `Remove one profile entry or every profile entry and optionally delete managed homes.

Usage: godex profile remove [OPTIONS] [NAME]

Options:
      --all          Remove every configured profile
      --delete-home  Also delete the managed CODEX_HOME directory
  -h, --help         Print help
`,
	"profile/use": `Set the active profile used by commands that omit --profile.

Usage: godex profile use [OPTIONS]

Options:
  -p, --profile NAME  Profile name
  -h, --help          Print help
`,
	"session": `Inspect shared Codex session metadata.

Usage: godex session <COMMAND>

Commands:
  list     List shared Codex sessions
  current  List shared Codex sessions started from the current directory
  resume   Resume a shared Codex session by unique partial or full id

Options:
  -h, --help  Print help
`,
	"session/list": `List shared Codex sessions.

Usage: godex session list [OPTIONS]

Options:
      --json               Emit machine-readable JSON
      --id-only            Print only full session ids
      --resume-command     Print a resume command for each matching session
      --profile NAME       Filter by profile binding
      --query TEXT         Filter by id, name, cwd, profile, or path
      --limit N            Limit results after newest-first sorting
      --include-subagents  Include spawned subagent sessions (compatibility flag)
      --parent-only        Show only resumable parent sessions
  -h, --help               Print help
`,
	"session/current": `List shared Codex sessions started from the current directory.

Usage: godex session current [OPTIONS]

Options:
      --json               Emit machine-readable JSON
      --id-only            Print only full session ids
      --resume-command     Print a resume command for each matching session
      --profile NAME       Filter by profile binding
      --query TEXT         Filter by id, name, cwd, profile, or path
      --limit N            Limit results after newest-first sorting
      --include-subagents  Include spawned subagent sessions (compatibility flag)
      --parent-only        Show only resumable parent sessions
      --cwd PATH           Godex extension: override the current-directory filter
  -h, --help               Print help
`,
	"session/resume": `Resume a shared Codex session by unique partial or full id.

Usage: godex session resume <ID>

Options:
  -h, --help  Print help
`,
	"ping": `Send lightweight prompt checks through ready profiles.

Usage: godex ping <COMMAND>

Commands:
  openai  Send a minimal application request through the OpenAI/Codex runtime path

Options:
  -h, --help  Print help
`,
	"ping/openai": `Send a minimal application request through the OpenAI/Codex runtime path.

Usage: godex ping openai [OPTIONS]

Options:
  -p, --profile NAME  Probe only this OpenAI profile
      --model MODEL   Model passed through the normal Codex request path
      --base-url URL  Override the ChatGPT backend base URL
      --no-proxy      Bypass upstream proxy settings
      --json          Emit stable JSON output
  -h, --help          Print help
`,
	"login": `Run provider login flows, using Godex profiles where supported.

Usage: godex login [OPTIONS] [PROFILE_OR_LOGIN_ARG]...

Arguments:
  [PROFILE_OR_LOGIN_ARG]...  Optional profile name first, followed by login-method flags or provider arguments

Options:
  -p, --profile NAME  Existing profile to log into. If omitted, Godex creates or reuses a profile by workspace identity
  -h, --help          Print help

Examples:
  godex login
  godex login main
  godex login --profile main
  godex login --device-auth
  godex login --with-claude
  godex login --with-antigravity

Notes:
  A leading non-option argument selects the profile; status remains Codex login status.
  Use --profile when selecting a profile literally named status.
  OpenAI/Codex, Claude, and API-key login paths create or update Godex profiles.
  Google Gemini OAuth profiles are unsupported; native Gemini CLI / Vertex AI compatibility is retired. Use a Gemini API key with godex s gemini; native Antigravity remains available through godex s gemini --cli agy.
  Antigravity login delegates to agy auth login and does not create a Godex profile.
`,
	"logout": `Run Codex logout for the selected or active Godex profile.

Usage: godex logout [OPTIONS] [NAME]

Arguments:
  [NAME]  Profile name. If omitted, Godex uses the active profile

Options:
  -p, --profile NAME  Profile name. If omitted, Godex uses the active profile
  -h, --help          Print help
`,
	"quota": `Inspect live quota for one profile or the whole profile pool.

Usage: godex quota [OPTIONS]

Options:
  -p, --profile NAME    Inspect one profile
      --all             Show every configured profile
      --auth AUTH       Filter by auth label or compatibility
      --provider NAME   Filter by provider
      --detail          Include reset timestamps and expanded window details
      --raw             Print raw usage JSON for one profile
      --once            Render one snapshot instead of refreshing
      --base-url URL    Override the upstream quota base URL
  -h, --help            Print help
`,
	"redeem": `Redeem one reset credit manually for a named OpenAI/Codex profile.

Usage: godex redeem [OPTIONS] <PROFILE>

Options:
  -y, --yes           Skip the near-reset confirmation prompt
      --base-url URL  Override the ChatGPT backend base URL
      --no-proxy      Bypass upstream proxy settings
  -h, --help          Print help
`,
	"status": `Monitor profiles, quota resets, token <redacted>, and Godex resource usage.

Usage: godex status [OPTIONS]

Options:
      --once              Render one snapshot instead of the live dashboard
      --interval SECONDS  Resource sampling interval in seconds
  -h, --help              Print help
`,
	"info": `Summarize Godex version, profiles, runtime policy, logs, and runtime state.

Usage: godex info [OPTIONS]

Options:
      --json    Emit machine-readable JSON
      --tokens  Include token <redacted> totals parsed from recent runtime logs
  -h, --help    Print help
`,
	"log": `Follow redacted Godex runtime logs.

Usage: godex log [OPTIONS] [MODE]

Modes:
  stream    Follow redacted runtime logs (default)
  last      Print the latest matching runtime-log line and exit
  upstream  Follow upstream request/response log events

Options:
      --json  Emit one JSON object per line
  -h, --help  Print help
`,
	"doctor": `Inspect local state, Codex resolution, quota readiness, and runtime logs.

Usage: godex doctor [OPTIONS]

Options:
      --quota
      --runtime
      --install
      --repair-import-auth-journals
      --repair-session-index
      --tail-bytes BYTES
      --suggest-policy
      --json
      --bundle [PATH]
      --redacted
  -h, --help  Print help
`,
	"gateway": `Run a lean OpenAI-compatible provider gateway.

Usage: godex gateway [OPTIONS]

Options:
      --listen ADDR
      --provider PROVIDER
      --base-url URL       Upstream base URL; alias: --url
      --api-key KEY
      --smart-context
      --presidio
      --no-presidio
  -h, --help               Print help
`,
	"update": `Update Godex from the latest verified GitHub release binary.

Usage: godex update

Options:
  -h, --help  Print help
`,
	"current": `Show the active profile and its CODEX_HOME details.

Usage: godex current

Options:
  -h, --help  Print help
`,
	"use": `Set the active profile used by commands that omit --profile.

Usage: godex use [OPTIONS]

Options:
  -p, --profile NAME  Profile name
  -h, --help          Print help
`,
}

func printPublicCommandHelp(out io.Writer, arguments []string) (bool, error) {
	key, ok := publicHelpKey(arguments)
	if !ok {
		return false, nil
	}
	if key == "run" {
		return true, runtimecli.PrintRunHelp(out)
	}
	if key == "super" {
		return true, runtimecli.PrintSuperHelp(out)
	}
	text, ok := publicHelpText[key]
	if !ok {
		return false, nil
	}
	if out == nil {
		return true, fmt.Errorf("help output is not configured")
	}
	_, err := io.WriteString(out, text)
	return true, err
}

func publicHelpKey(arguments []string) (string, bool) {
	if len(arguments) == 0 {
		return "", false
	}
	if arguments[0] == "help" {
		return publicHelpPath(arguments[1:])
	}

	switch arguments[0] {
	case "run":
		if runtimecli.RunHelpRequested(arguments[1:]) {
			return "run", true
		}
		return "", false
	case "super", "s":
		if runtimecli.SuperHelpRequested(arguments[1:]) {
			return "super", true
		}
		return "", false
	case "profile":
		return nestedPublicHelp(arguments, "profile", map[string]bool{
			"add": true, "export": true, "import": true, "import-current": true,
			"list": true, "remove": true, "use": true,
		})
	case "session":
		return nestedPublicHelp(arguments, "session", map[string]bool{
			"list": true, "current": true, "resume": true,
		})
	case "ping":
		return nestedPublicHelp(arguments, "ping", map[string]bool{"openai": true})
	case "quota", "redeem", "status", "info", "log", "doctor", "gateway",
		"update", "current", "use", "login", "logout":
		if helpFlagBeforeDelimiter(arguments[1:]) {
			return arguments[0], true
		}
	}
	return "", false
}

func nestedPublicHelp(arguments []string, root string, children map[string]bool) (string, bool) {
	if len(arguments) < 2 {
		return "", false
	}
	if arguments[1] == "help" {
		if len(arguments) == 2 {
			return root, true
		}
		if children[arguments[2]] {
			return root + "/" + arguments[2], true
		}
		return "", false
	}
	if arguments[1] == "--help" || arguments[1] == "-h" {
		return root, true
	}
	if children[arguments[1]] && helpFlagBeforeDelimiter(arguments[2:]) {
		return root + "/" + arguments[1], true
	}
	return "", false
}

func publicHelpPath(path []string) (string, bool) {
	if len(path) == 0 {
		return "", false
	}
	switch path[0] {
	case "run":
		return "run", true
	case "super", "s":
		return "super", true
	case "profile":
		if len(path) == 1 {
			return "profile", true
		}
		if _, ok := publicHelpText["profile/"+path[1]]; ok {
			return "profile/" + path[1], true
		}
	case "session":
		if len(path) == 1 {
			return "session", true
		}
		if _, ok := publicHelpText["session/"+path[1]]; ok {
			return "session/" + path[1], true
		}
	case "ping":
		if len(path) == 1 {
			return "ping", true
		}
		if path[1] == "openai" {
			return "ping/openai", true
		}
	default:
		if _, ok := publicHelpText[path[0]]; ok {
			return path[0], true
		}
	}
	return "", false
}

func helpFlagBeforeDelimiter(arguments []string) bool {
	for _, argument := range arguments {
		if argument == "--" {
			return false
		}
		if argument == "--help" || argument == "-h" {
			return true
		}
	}
	return false
}

func publicHelpRequested(arguments []string) bool {
	_, ok := publicHelpKey(arguments)
	return ok
}
