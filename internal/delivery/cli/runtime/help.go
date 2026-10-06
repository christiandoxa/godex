package runtime

import (
	"fmt"
	"io"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

const runHelpText = `Run Codex through Godex with quota preflight and eligible pre-commit rotation.

Usage: godex run [OPTIONS] [CODEX_ARG]...

Arguments:
  [CODEX_ARG]...  Arguments passed through to Codex. A lone session id is normalized to Codex resume.

Options:
  -p, --profile NAME                 Starting profile; defaults to the active profile
      --account SELECTOR             Select a managed Godex account
      --auto-rotate                  Allow eligible pre-commit rotation (default)
      --no-auto-rotate               Keep the selected profile fixed
      --auto-redeem                  Allow one eligible reset-credit redemption
      --skip-quota-check             Skip launch-time quota preflight
      --full-access                  Pass Codex's sandbox/approval bypass launch flag
      --base-url URL                 Override the OpenAI/ChatGPT upstream for quota and runtime proxy
      --no-proxy                     Disable upstream proxy environment/system proxy use
      --dry-run                      Print resolved launch diagnostics without starting Codex
      --web-search MODE              disabled, cached, indexed, or live
      --rollout-budget-tokens N      Enable rollout-budget reminders
      --rollout-budget-reminders N   Comma-separated remaining-token reminder points
      --rollout-budget-sampling-weight N
      --rollout-budget-prefill-weight N
      --current-time-reminder
      --current-time-reminder-interval N
      --current-time-clock-source SOURCE
      --respect-system-proxy
      --no-respect-system-proxy
      --provider PROVIDER            Godex provider bridge shortcut
      --api-key KEY                  Explicit provider API key
      --url URL                      Local OpenAI-compatible endpoint
      --model MODEL                  Model override
      --context-window TOKENS
      --auto-compact-token-limit TOKENS
      --cli agy                      Native Antigravity compatibility path
  -h, --help                         Print help

Notes:
  Wrapper options are consumed only before the Codex argument boundary.
  After the first Codex command/positional argument, child flags such as --help are forwarded to Codex.
`

const superHelpText = `YOLO shortcut for the Godex Super tool stack with opt-in Presidio.

Usage: godex super [OPTIONS] [CODEX_ARG]...

Arguments:
  [CODEX_ARG]...  Arguments passed through to Codex after Godex-generated options.

Options:
  -p, --profile NAME
      --auto-rotate
      --no-auto-rotate
      --auto-redeem
      --skip-quota-check
      --full-access                  Compatibility flag; Super is already full-access
      --dry-run
      --base-url URL
      --no-proxy
      --presidio
      --no-presidio
      --sub-agent
      --no-sub-agent
      --sub-agent-provider PROVIDER
      --sub-agent-model MODEL
      --sub-agent-model-reasoning-effort EFFORT
      --sub-agent-url URL
      --sub-agent-max-concurrency VALUE
      --tool TOOL
      --require-tool TOOL
      --url URL
      --provider PROVIDER
      --cli agy
      --api-key KEY
      --model MODEL                  Alias: --local-model
      --context-window TOKENS        Alias: --local-context-window
      --auto-compact-token-limit TOKENS
                                      Alias: --local-auto-compact-token-limit
      --web-search MODE
      --rollout-budget-tokens N
      --rollout-budget-reminders N
      --rollout-budget-sampling-weight N
      --rollout-budget-prefill-weight N
      --current-time-reminder
      --current-time-reminder-interval N
      --current-time-clock-source SOURCE
      --respect-system-proxy
      --no-respect-system-proxy
  -h, --help                         Print help

Notes:
  Super enables Smart Context, full access, and available typed optimizer tools in a temporary Godex overlay.
  Missing optional tools are skipped unless required with --require-tool.
  Use --presidio or --no-presidio for explicit non-interactive Presidio policy.
  Child flags after the Codex command boundary are forwarded to Codex.
`

func PrintRunHelp(out io.Writer) error {
	if out == nil {
		return fmt.Errorf("run help output is not configured")
	}
	_, err := io.WriteString(out, runHelpText)
	return err
}

func PrintSuperHelp(out io.Writer) error {
	if out == nil {
		return fmt.Errorf("Super help output is not configured")
	}
	_, err := io.WriteString(out, superHelpText)
	return err
}

func RunHelpRequested(arguments []string) bool {
	selection := runtimemodel.Selection{}
	features := runtimeFeatures{}
	for index := 0; index < len(arguments); {
		argument := arguments[index]
		if argument == "--" {
			return false
		}
		if argument == "--help" || argument == "-h" {
			return true
		}
		next, handled, err := consumeWrapperArgument(arguments, index, &selection, &features)
		if err != nil || !handled {
			return false
		}
		index = next
	}
	return false
}

func SuperHelpRequested(arguments []string) bool {
	options := superOptions{}
	arguments = rewriteSuperProviderAlias(arguments)
	tailMode := false
	for index := 0; index < len(arguments); {
		argument := arguments[index]
		if argument == "--" {
			return false
		}
		if argument == "--help" || argument == "-h" {
			return !tailMode
		}
		next, handled, err := consumeSuperArgument(arguments, index, &options, tailMode)
		if err != nil {
			return false
		}
		if handled {
			index = next
			continue
		}
		tailMode = true
		index++
	}
	return false
}
