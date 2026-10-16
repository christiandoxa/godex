package runtime

import "slices"

// The tagged Prodex 0.438.0 parser first checks for a literal `--` boundary.
// A provider-looking string in a literal native Codex tail is not an alias.
// The existing prefix rewrite still handles the historical shorthand with
// leading provider name before any other Super options.
func superAliasArguments04380(arguments []string) ([]string, bool) {
	literal := slices.Contains(arguments, "--")
	if literal {
		return arguments, true
	}
	return rewriteSuperProviderAlias(arguments), false
}

// `rewrite_positioned_super_alias` executes *after* the CLI parses its own
// leading options. If the first remaining Codex input is a provider alias,
// promote it unless another provider/URL or literal boundary owns the tail.
// No client argv bytes are changed apart from removing that exact alias.
func superSelectPositionedProvider04380(options *superOptions, literal bool) {
	if options == nil || literal || options.provider != "" || options.localURL != "" || len(options.codexArgs) == 0 {
		return
	}
	switch options.codexArgs[0] {
	case "gemini", "deepseek":
		options.provider = options.codexArgs[0]
		options.codexArgs = options.codexArgs[1:]
	}
}

func rewriteSuperProviderAlias(arguments []string) []string {
	if len(arguments) == 0 {
		return arguments
	}
	if arguments[0] != "gemini" && arguments[0] != "deepseek" {
		return arguments
	}
	result := make([]string, 0, len(arguments)+1)
	result = append(result, "--provider", arguments[0])
	result = append(result, arguments[1:]...)
	return result
}
