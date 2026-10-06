package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"
)

var superDefaultTools = []string{
	"caveman",
	"rtk",
	"codebase-memory-mcp",
	"playwright-mcp",
	"ponytail",
}

type superSubAgent struct {
	enabled              bool
	disabled             bool
	detailConfigured     bool
	provider             string
	model                string
	effort               string
	url                  string
	maxConcurrency       uint16
	maxConcurrencySource string
}

type superOptions struct {
	profile            string
	autoRotate         bool
	noAutoRotate       bool
	directAutoRotate   bool
	directNoAutoRotate bool
	autoRedeem         bool
	skipQuota          bool
	dryRun             bool
	noProxy            bool
	presidio           bool
	noPresidio         bool
	provider           string
	cli                string
	apiKey             string
	baseURL            string
	localURL           string
	model              string
	contextWindow      *uint64
	autoCompact        *uint64
	tools              []string
	requiredTools      []string
	subAgent           superSubAgent
	features           runtimeFeatures
	codexArgs          []string
	fullAccess         bool
	smartContext       bool
	superMode          bool
	resolvedTools      []superToolStatus
	toolResolutionDone bool
}

type superToolStatus struct {
	name     string
	resolved bool
	path     string
	required bool
	service  bool
}

type superToolLookup func(string) (string, bool)

func Super(ctx context.Context, out io.Writer, arguments []string) error {
	return superWithToolLookup(ctx, out, arguments, defaultSuperToolLookup)
}

func superWithToolLookup(_ context.Context, out io.Writer, arguments []string, lookup superToolLookup) error {
	if SuperHelpRequested(arguments) {
		return PrintSuperHelp(out)
	}
	options, err := parseSuperArguments(arguments)
	if err != nil {
		return err
	}
	tools, err := resolveSuperTools(options, lookup)
	if err != nil {
		return err
	}
	options.resolvedTools = tools
	options.toolResolutionDone = true
	if !options.dryRun {
		return errors.New("Godex Super runtime dependencies are required for non-dry-run launch")
	}
	return renderSuperDryRunResolved(out, options, tools)
}

func SuperProfiles(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	sessions *sessionusecase.Catalog,
	profiles launchProfiles,
	out io.Writer,
	arguments []string,
) error {
	return superProfilesWithToolLookup(
		ctx, runner, sessions, profiles, out, arguments, defaultSuperToolLookup,
	)
}

func superProfilesWithToolLookup(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	sessions *sessionusecase.Catalog,
	profiles launchProfiles,
	out io.Writer,
	arguments []string,
	lookup superToolLookup,
) error {
	if SuperHelpRequested(arguments) {
		return PrintSuperHelp(out)
	}
	options, err := parseSuperArguments(arguments)
	if err != nil {
		return err
	}
	tools, err := resolveSuperTools(options, lookup)
	if err != nil {
		return err
	}
	options.resolvedTools = tools
	options.toolResolutionDone = true
	if options.dryRun {
		return renderSuperDryRunResolved(out, options, tools)
	}
	return launchSuperProfiles(ctx, runner, sessions, profiles, options)
}

func parseSuperArguments(arguments []string) (superOptions, error) {
	options := superOptions{
		fullAccess:   true,
		smartContext: true,
		superMode:    true,
		subAgent: superSubAgent{
			provider:             "openai",
			maxConcurrency:       4,
			maxConcurrencySource: "default",
		},
	}
	arguments = rewriteSuperProviderAlias(arguments)
	tailMode := false
	for index := 0; index < len(arguments); {
		if arguments[index] == "--" {
			options.codexArgs = append(options.codexArgs, arguments[index:]...)
			break
		}
		next, handled, err := consumeSuperArgument(arguments, index, &options, tailMode)
		if err != nil {
			return superOptions{}, err
		}
		if handled {
			index = next
			continue
		}
		options.codexArgs = append(options.codexArgs, arguments[index])
		tailMode = true
		index++
	}
	if err := validateSuperOptions(options); err != nil {
		return superOptions{}, err
	}
	if options.provider != "" || options.localURL != "" {
		options.skipQuota = true
	}
	featureArgs, err := options.features.arguments()
	if err != nil {
		return superOptions{}, err
	}
	options.codexArgs = append(featureArgs, options.codexArgs...)
	if options.provider == "" && options.localURL == "" {
		options.codexArgs = append([]string{"-c", "features.apps=false"}, options.codexArgs...)
	}
	if options.model != "" && options.provider == "" && options.localURL == "" {
		options.codexArgs = append([]string{"-c", "model=" + strconv.Quote(options.model)}, options.codexArgs...)
	}
	return options, nil
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

func consumeSuperArgument(arguments []string, index int, options *superOptions, tailMode bool) (int, bool, error) {
	argument := arguments[index]
	switch argument {
	case "--auto-rotate":
		if !tailMode {
			options.directAutoRotate = true
		}
		options.autoRotate, options.noAutoRotate = true, false
		return index + 1, true, nil
	case "--no-auto-rotate":
		if !tailMode {
			options.directNoAutoRotate = true
		}
		options.noAutoRotate, options.autoRotate = true, false
		return index + 1, true, nil
	case "--auto-redeem":
		options.autoRedeem = true
		return index + 1, true, nil
	case "--skip-quota-check":
		options.skipQuota = true
		return index + 1, true, nil
	case "--dry-run":
		options.dryRun = true
		return index + 1, true, nil
	case "--no-proxy":
		options.noProxy = true
		return index + 1, true, nil
	case "--presidio":
		options.presidio = true
		return index + 1, true, nil
	case "--no-presidio":
		options.noPresidio = true
		return index + 1, true, nil
	case "--sub-agent":
		options.subAgent.enabled = true
		return index + 1, true, nil
	case "--no-sub-agent":
		options.subAgent.disabled = true
		return index + 1, true, nil
	case "--full-access":
		options.fullAccess = true
		return index + 1, true, nil
	case "--current-time-reminder":
		options.features.currentTime = true
		return index + 1, true, nil
	case "--respect-system-proxy":
		value := true
		options.features.respectSystemProxy = &value
		return index + 1, true, nil
	case "--no-respect-system-proxy":
		value := false
		options.features.respectSystemProxy = &value
		return index + 1, true, nil
	}

	if value, consumed, ok, err := namedOptionValue(arguments, index, "--profile"); ok {
		if err != nil {
			return index, true, err
		}
		if options.profile == "" {
			options.profile = value
		}
		return index + consumed, true, nil
	}
	if value, consumed, ok, err := namedOptionValue(arguments, index, "-p"); ok {
		if err != nil {
			return index, true, err
		}
		if options.profile == "" {
			options.profile = value
		}
		return index + consumed, true, nil
	}
	if value, consumed, ok, err := namedOptionValue(arguments, index, "--provider"); ok {
		if err != nil {
			return index, true, err
		}
		provider, err := normalizeExternalProvider(value)
		if err != nil {
			return index, true, err
		}
		options.provider = provider
		return index + consumed, true, nil
	}
	if value, consumed, ok, err := namedOptionValue(arguments, index, "--cli"); ok {
		if err != nil {
			return index, true, err
		}
		if strings.TrimSpace(value) != "agy" {
			return index, true, errors.New("only --cli agy is retained; use --provider for other providers")
		}
		options.cli = "agy"
		return index + consumed, true, nil
	}
	if value, consumed, ok, err := namedSecretOptionValue(arguments, index, "--api-key"); ok {
		if err != nil {
			return index, true, err
		}
		options.apiKey = value
		return index + consumed, true, nil
	}
	for _, option := range []struct {
		name string
		set  func(string) error
	}{
		{"--base-url", func(value string) error {
			if err := validateCredentialFreeHTTPURL(value, "--base-url"); err != nil {
				return err
			}
			options.baseURL = value
			return nil
		}},
		{"--url", func(value string) error {
			if err := validateCredentialFreeHTTPURL(value, "--url"); err != nil {
				return err
			}
			options.localURL = value
			return nil
		}},
		{"--model", func(value string) error { options.model = value; return nil }},
		{"--local-model", func(value string) error { options.model = value; return nil }},
	} {
		if value, consumed, ok, err := namedOptionValue(arguments, index, option.name); ok {
			if err != nil {
				return index, true, err
			}
			if err := option.set(value); err != nil {
				return index, true, err
			}
			return index + consumed, true, nil
		}
	}
	for _, option := range []struct {
		name string
		dest **uint64
	}{
		{"--context-window", &options.contextWindow},
		{"--local-context-window", &options.contextWindow},
		{"--auto-compact-token-limit", &options.autoCompact},
		{"--local-auto-compact-token-limit", &options.autoCompact},
	} {
		if value, consumed, ok, err := namedOptionValue(arguments, index, option.name); ok {
			if err != nil {
				return index, true, err
			}
			parsed, err := parseProviderUint(value, option.name)
			if err != nil {
				return index, true, err
			}
			*option.dest = &parsed
			return index + consumed, true, nil
		}
	}
	for _, option := range []struct {
		name     string
		required bool
	}{
		{"--tool", false},
		{"--require-tool", true},
	} {
		if value, consumed, ok, err := namedOptionValue(arguments, index, option.name); ok {
			if err != nil {
				return index, true, err
			}
			tool, err := normalizeSuperTool(value)
			if err != nil {
				return index, true, err
			}
			options.tools = appendUnique(options.tools, tool)
			if option.required {
				options.requiredTools = appendUnique(options.requiredTools, tool)
			}
			return index + consumed, true, nil
		}
	}
	if next, handled, err := consumeSuperSubAgentArgument(arguments, index, options); handled {
		return next, true, err
	}
	if next, handled, err := consumeSuperFeatureArgument(arguments, index, options); handled {
		return next, true, err
	}
	return index, false, nil
}

func consumeSuperSubAgentArgument(arguments []string, index int, options *superOptions) (int, bool, error) {
	if value, consumed, ok, err := namedOptionValue(arguments, index, "--sub-agent-provider"); ok {
		if err != nil {
			return index, true, err
		}
		provider, err := normalizeSubAgentProvider(value)
		if err != nil {
			return index, true, err
		}
		options.subAgent.detailConfigured = true
		options.subAgent.provider = provider
		return index + consumed, true, nil
	}
	if value, consumed, ok, err := namedOptionValue(arguments, index, "--sub-agent-model"); ok {
		if err != nil {
			return index, true, err
		}
		if strings.TrimSpace(value) == "" {
			return index, true, errors.New("--sub-agent-model must be nonempty")
		}
		options.subAgent.detailConfigured = true
		options.subAgent.model = value
		return index + consumed, true, nil
	}
	if value, consumed, ok, err := namedOptionValue(arguments, index, "--sub-agent-model-reasoning-effort"); ok {
		if err != nil {
			return index, true, err
		}
		effort := strings.ToLower(strings.TrimSpace(value))
		if !slices.Contains([]string{"none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra"}, effort) {
			return index, true, fmt.Errorf("invalid sub-agent reasoning effort %q", effort)
		}
		options.subAgent.detailConfigured = true
		options.subAgent.effort = effort
		return index + consumed, true, nil
	}
	if value, consumed, ok, err := namedOptionValue(arguments, index, "--sub-agent-url"); ok {
		if err != nil {
			return index, true, err
		}
		if err := validateCredentialFreeHTTPURL(value, "--sub-agent-url"); err != nil {
			return index, true, err
		}
		options.subAgent.detailConfigured = true
		options.subAgent.url = value
		return index + consumed, true, nil
	}
	if value, consumed, ok, err := namedOptionValue(arguments, index, "--sub-agent-max-concurrency"); ok {
		if err != nil {
			return index, true, err
		}
		parsed, err := parseSubAgentConcurrency(value)
		if err != nil {
			return index, true, err
		}
		options.subAgent.detailConfigured = true
		options.subAgent.maxConcurrency = parsed
		if strings.EqualFold(strings.TrimSpace(value), "default") {
			options.subAgent.maxConcurrencySource = "default"
		} else {
			options.subAgent.maxConcurrencySource = "custom"
		}
		return index + consumed, true, nil
	}
	return index, false, nil
}

func consumeSuperFeatureArgument(arguments []string, index int, options *superOptions) (int, bool, error) {
	switch featureName(arguments[index]) {
	case "--web-search":
		value, consumed, _, err := featureValue(arguments, index, "--web-search")
		if err != nil {
			return index, true, err
		}
		value = strings.ToLower(value)
		if !slices.Contains([]string{"disabled", "cached", "indexed", "live"}, value) {
			return index, true, fmt.Errorf("invalid --web-search value %q", value)
		}
		options.features.webSearch = value
		return index + consumed, true, nil
	case rolloutBudgetTokensFlag:
		value, consumed, _, prior := featureValue(arguments, index, rolloutBudgetTokensFlag)
		parsed, err := parseUintFeature(rolloutBudgetTokensFlag, value, prior)
		if err != nil {
			return index, true, err
		}
		options.features.rolloutLimit = &parsed
		return index + consumed, true, nil
	case rolloutBudgetRemindersFlag:
		value, consumed, _, prior := featureValue(arguments, index, rolloutBudgetRemindersFlag)
		parsed, err := parseUintListFeature(rolloutBudgetRemindersFlag, value, prior)
		if err != nil {
			return index, true, err
		}
		options.features.rolloutReminders = append(options.features.rolloutReminders, parsed...)
		return index + consumed, true, nil
	case rolloutBudgetSamplingWeightFlag:
		value, consumed, _, prior := featureValue(arguments, index, rolloutBudgetSamplingWeightFlag)
		parsed, err := parseFloatFeature(rolloutBudgetSamplingWeightFlag, value, prior)
		if err != nil {
			return index, true, err
		}
		options.features.samplingWeight = &parsed
		return index + consumed, true, nil
	case rolloutBudgetPrefillWeightFlag:
		value, consumed, _, prior := featureValue(arguments, index, rolloutBudgetPrefillWeightFlag)
		parsed, err := parseFloatFeature(rolloutBudgetPrefillWeightFlag, value, prior)
		if err != nil {
			return index, true, err
		}
		options.features.prefillWeight = &parsed
		return index + consumed, true, nil
	case currentTimeReminderIntervalFlag:
		value, consumed, _, prior := featureValue(arguments, index, currentTimeReminderIntervalFlag)
		parsed, err := parseUintFeature(currentTimeReminderIntervalFlag, value, prior)
		if err != nil {
			return index, true, err
		}
		options.features.currentInterval = &parsed
		return index + consumed, true, nil
	case "--current-time-clock-source":
		value, consumed, _, err := featureValue(arguments, index, "--current-time-clock-source")
		if err != nil {
			return index, true, err
		}
		value = strings.ToLower(value)
		if value != "system" && value != "external" {
			return index, true, fmt.Errorf("invalid --current-time-clock-source value %q", value)
		}
		options.features.currentClock = value
		return index + consumed, true, nil
	default:
		return index, false, nil
	}
}

func validateSuperOptions(options superOptions) error {
	if _, active := os.LookupEnv("GODEX_SUB_AGENT"); active && options.subAgent.enabled {
		return errors.New("--sub-agent cannot be re-enabled while GODEX_SUB_AGENT is set")
	}
	if options.directAutoRotate && options.directNoAutoRotate {
		return errors.New("--auto-rotate conflicts with --no-auto-rotate")
	}
	if options.presidio && options.noPresidio {
		return errors.New("--presidio conflicts with --no-presidio")
	}
	if options.noPresidio && slices.Contains(options.requiredTools, "presidio") {
		return errors.New("--no-presidio conflicts with --require-tool presidio")
	}
	if options.provider != "" && options.localURL != "" {
		return errors.New("--provider conflicts with --url")
	}
	if options.baseURL != "" && options.localURL != "" {
		return errors.New("--base-url conflicts with --url")
	}
	if options.apiKey != "" && options.provider == "" {
		return errors.New("--api-key requires --provider")
	}
	if (options.contextWindow != nil || options.autoCompact != nil) && options.provider == "" && options.localURL == "" {
		return errors.New("context-window options require --provider or --url")
	}
	if options.subAgent.enabled && options.subAgent.disabled {
		return errors.New("--sub-agent conflicts with --no-sub-agent")
	}
	if !options.subAgent.enabled && options.subAgent.detailConfigured {
		return errors.New("sub-agent detail flags require explicit --sub-agent")
	}
	if options.subAgent.provider == "local" && options.subAgent.enabled && options.subAgent.url == "" {
		return errors.New("local sub-agent provider requires --sub-agent-url")
	}
	if options.subAgent.provider != "local" && options.subAgent.url != "" {
		return errors.New("--sub-agent-url requires --sub-agent-provider local")
	}
	if options.subAgent.enabled && len(options.codexArgs) > 0 && options.codexArgs[0] == "gui" {
		return errors.New("--sub-agent is unsupported with the Codex Desktop frontend")
	}
	if options.cli != "" && options.provider != "gemini" {
		return errors.New("--cli agy requires --provider gemini")
	}
	return nil
}

func renderSuperDryRun(out io.Writer, options superOptions, lookup superToolLookup) error {
	tools := options.resolvedTools
	if !options.toolResolutionDone {
		var err error
		tools, err = resolveSuperTools(options, lookup)
		if err != nil {
			return err
		}
	}
	return renderSuperDryRunResolved(out, options, tools)
}

func renderSuperDryRunResolved(out io.Writer, options superOptions, tools []superToolStatus) error {
	profile := "(active/default)"
	if options.profile != "" {
		profile = options.profile
	}
	provider := "openai"
	if options.provider != "" {
		provider = options.provider
	} else if options.localURL != "" {
		provider = "local"
	}
	fmt.Fprintln(out, "Godex Super dry run")
	fmt.Fprintf(out, "Profile: %s\n", profile)
	fmt.Fprintf(out, "Provider: %s\n", provider)
	fmt.Fprintf(out, "Auto rotate: %s\n", enabledLabel(!options.noAutoRotate))
	fmt.Fprintf(out, "Auto redeem: %s\n", enabledLabel(options.autoRedeem))
	fmt.Fprintf(out, "Quota preflight: %s\n", skippedLabel(options.skipQuota))
	fmt.Fprintln(out, "Full access: enabled")
	fmt.Fprintln(out, "Smart Context: enabled")
	fmt.Fprintf(out, "Presidio redaction: %s\n", enabledLabel(superPresidioEnabled(options)))
	if options.apiKey != "" {
		fmt.Fprintln(out, "Provider API key: configured (<redacted>)")
	}
	if options.baseURL != "" {
		fmt.Fprintf(out, "Provider base URL: %s\n", options.baseURL)
	}
	if options.localURL != "" {
		fmt.Fprintf(out, "Local URL: %s\n", options.localURL)
	}
	if options.model != "" {
		fmt.Fprintf(out, "Model: %s\n", options.model)
	}
	fmt.Fprintln(out, "Optional tools:")
	for _, status := range tools {
		state := "skipped (not found)"
		if status.service {
			state = "requested (service activation deferred until launch)"
		} else if status.resolved {
			state = "resolved (activation deferred until launch)"
		}
		required := ""
		if status.required {
			required = " [required]"
		}
		fmt.Fprintf(out, "  %s%s: %s\n", status.name, required, state)
	}
	if options.subAgent.enabled {
		fmt.Fprintf(out, "Sub-agent: enabled provider=%s max_concurrency=%d", options.subAgent.provider, options.subAgent.maxConcurrency)
		if options.subAgent.model != "" {
			fmt.Fprintf(out, " model=%s", options.subAgent.model)
		}
		if options.subAgent.effort != "" {
			fmt.Fprintf(out, " effort=%s", options.subAgent.effort)
		}
		if options.subAgent.url != "" {
			fmt.Fprintf(out, " url=%s", options.subAgent.url)
		}
		fmt.Fprintln(out)
	} else {
		fmt.Fprintln(out, "Sub-agent: disabled")
	}
	fmt.Fprintf(out, "Codex args: %s\n", renderRedactedArgs(superPreparedCodexArgs(options)))
	fmt.Fprintln(out, "Dry run: overlays and services are not started.")
	return nil
}

func resolveSuperTools(options superOptions, lookup superToolLookup) ([]superToolStatus, error) {
	selected := append([]string(nil), superDefaultTools...)
	for _, tool := range options.tools {
		if tool == "presidio" && options.noPresidio && !slices.Contains(options.requiredTools, tool) {
			continue
		}
		selected = appendUnique(selected, tool)
	}
	if superPresidioEnabled(options) {
		selected = appendUnique(selected, "presidio")
	}
	statuses := make([]superToolStatus, 0, len(selected))
	for _, tool := range selected {
		status := superToolStatus{
			name:     tool,
			required: slices.Contains(options.requiredTools, tool),
			service:  tool == "presidio",
		}
		if !status.service {
			status.path, status.resolved = lookup(tool)
		}
		if status.required && !status.service && !status.resolved {
			return nil, fmt.Errorf("required optional tool %s is unavailable; run godex doctor --install", tool)
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func normalizeSuperTool(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "caveman":
		return "caveman", nil
	case "rtk":
		return "rtk", nil
	case "codebase-memory-mcp", "codebase-memory", "cbm":
		return "codebase-memory-mcp", nil
	case "playwright", "playwright-mcp":
		return "playwright-mcp", nil
	case "ponytail":
		return "ponytail", nil
	case "presidio":
		return "presidio", nil
	default:
		return "", fmt.Errorf("unknown optional tool %s", value)
	}
}

func normalizeSubAgentProvider(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "openai":
		return "openai", nil
	case "anthropic", "claude":
		return "anthropic", nil
	case "copilot", "github-copilot", "github_copilot":
		return "copilot", nil
	case "deepseek":
		return "deepseek", nil
	case "gemini", "google":
		return "gemini", nil
	case "kiro":
		return "kiro", nil
	case "local":
		return "local", nil
	default:
		return "", fmt.Errorf("invalid --sub-agent-provider: supported values are openai, anthropic, copilot, deepseek, gemini, kiro, local, got %q", value)
	}
}

func parseSubAgentConcurrency(value string) (uint16, error) {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "default") {
		return 4, nil
	}
	parsed, err := strconv.ParseUint(value, 10, 16)
	if err != nil {
		return 0, errors.New("invalid maximum active sub-agents: expected default or an integer from 1 to 64")
	}
	if parsed < 1 || parsed > 64 {
		return 0, errors.New("maximum active sub-agents must be between 1 and 64")
	}
	return uint16(parsed), nil
}

func superPresidioEnabled(options superOptions) bool {
	return slices.Contains(options.requiredTools, "presidio") ||
		(options.presidio && !options.noPresidio) ||
		(slices.Contains(options.tools, "presidio") && !options.noPresidio)
}

func superPresidioRequired(options superOptions) bool {
	return slices.Contains(options.requiredTools, "presidio")
}

func superPreparedCodexArgs(options superOptions) []string {
	return ensureSuperFullAccess(options.codexArgs)
}

func ensureSuperFullAccess(arguments []string) []string {
	for _, argument := range arguments {
		if argument == "--dangerously-bypass-approvals-and-sandbox" {
			return arguments
		}
	}
	return append([]string{"--dangerously-bypass-approvals-and-sandbox"}, arguments...)
}

func appendUnique(values []string, value string) []string {
	if !slices.Contains(values, value) {
		return append(values, value)
	}
	return values
}

func enabledLabel(value bool) string {
	if value {
		return "enabled"
	}
	return "disabled"
}

func skippedLabel(skipped bool) string {
	if skipped {
		return "skipped"
	}
	return "enabled"
}

func renderRedactedArgs(arguments []string) string {
	if len(arguments) == 0 {
		return "(none)"
	}
	return strings.Join(arguments, " ")
}
