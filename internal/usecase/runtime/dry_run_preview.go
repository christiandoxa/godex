package runtime

// DryRunBinaryLabel returns a non-sensitive label for the configured Codex
// child without resolving, probing, or starting it.
func (runner *Runner) DryRunBinaryLabel() string {
	if runner == nil || runner.process == nil {
		return "codex"
	}
	if preview, ok := runner.process.(interface{ RuntimeDryRunBinaryLabel() string }); ok {
		if label := preview.RuntimeDryRunBinaryLabel(); label != "" {
			return label
		}
	}
	return "codex"
}

// PreviewLocalProviderArguments projects the same direct local-provider Codex
// arguments used by a real launch without starting a child.
func PreviewLocalProviderArguments(config LocalProviderConfig, arguments []string) ([]string, error) {
	return localProviderArguments(config, arguments)
}

// PreviewOpenAICompatibleArguments projects the same direct profile rewrite
// used by a real OpenAI-compatible profile launch.
func PreviewOpenAICompatibleArguments(baseURL string, arguments []string) ([]string, error) {
	return openAICompatibleArguments(baseURL, arguments)
}
