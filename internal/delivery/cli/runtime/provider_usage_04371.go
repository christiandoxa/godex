package runtime

import "fmt"

// An unsupported external-provider preset is a CLI argument error, not a
// runtime/backend failure. The tagged Prodex 0.437.1 clap parser returns
// exit 2 for the same unsupported --provider value.
type unsupportedProviderArgument struct{ value string }

func (err unsupportedProviderArgument) Error() string {
	return fmt.Sprintf(
		"invalid --provider: supported values are anthropic, copilot, deepseek, gemini, kiro, got %q",
		err.value,
	)
}
func (unsupportedProviderArgument) ExitCode() int          { return 2 }
func (unsupportedProviderArgument) CLIArgumentError() bool { return true }
