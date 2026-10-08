package ping

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	pingmodel "github.com/christiandoxa/godex/internal/model/ping"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

const maxPingConcurrency = 4

type profileSource interface {
	QuotaTargets(context.Context) ([]profilemodel.QuotaTarget, error)
}

type processGateway interface {
	PingOpenAI(context.Context, pingmodel.Target, pingmodel.Options) (pingmodel.ProcessResult, error)
}

type OpenAI struct {
	profiles profileSource
	process  processGateway
	now      func() time.Time
}

func NewOpenAI(profiles profileSource, process processGateway) *OpenAI {
	return &OpenAI{profiles: profiles, process: process, now: time.Now}
}

func (ping *OpenAI) Run(ctx context.Context, options pingmodel.Options) (pingmodel.Report, error) {
	return ping.RunObserved(ctx, options, nil)
}

func (ping *OpenAI) RunObserved(
	ctx context.Context,
	options pingmodel.Options,
	observe func(pingmodel.Result) error,
) (pingmodel.Report, error) {
	if ping == nil || ping.profiles == nil || ping.process == nil {
		return pingmodel.Report{}, errors.New("OpenAI ping support is not configured")
	}
	if err := validateOptions(options); err != nil {
		return pingmodel.Report{}, err
	}
	normalizedEffort, err := normalizePingEffort(options.Model, options.Effort)
	if err != nil {
		return pingmodel.Report{}, err
	}
	options.Effort = normalizedEffort
	targets, err := ping.targets(ctx, options.Profile)
	if err != nil {
		return pingmodel.Report{}, err
	}
	started := ping.now()
	results, err := ping.probeTargets(ctx, targets, options, observe)
	if err != nil {
		return pingmodel.Report{}, err
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Profile < results[j].Profile })
	return buildReport(results, len(targets), ping.now().Sub(started)), nil
}

func (ping *OpenAI) targets(ctx context.Context, requested string) ([]pingmodel.Target, error) {
	profiles, err := ping.profiles.QuotaTargets(ctx)
	if err != nil {
		return nil, err
	}
	if requested != "" {
		for _, profile := range profiles {
			if profile.Name != requested {
				continue
			}
			if profile.Provider != "openai" {
				return nil, fmt.Errorf("profile %q is not configured for OpenAI", requested)
			}
			return []pingmodel.Target{{Name: profile.Name, CodexHome: profile.CodexHome}}, nil
		}
		return nil, fmt.Errorf("OpenAI profile %q is not configured", requested)
	}
	targets := make([]pingmodel.Target, 0, len(profiles))
	for _, profile := range profiles {
		if profile.Provider == "openai" {
			targets = append(targets, pingmodel.Target{Name: profile.Name, CodexHome: profile.CodexHome})
		}
	}
	return targets, nil
}

func (ping *OpenAI) probeTargets(
	ctx context.Context,
	targets []pingmodel.Target,
	options pingmodel.Options,
	observe func(pingmodel.Result) error,
) ([]pingmodel.Result, error) {
	if len(targets) == 0 {
		return nil, nil
	}
	probeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	workers := min(len(targets), min(maxPingConcurrency, max(1, runtime.GOMAXPROCS(0))))
	jobs := make(chan pingmodel.Target)
	results := make(chan pingmodel.Result, len(targets))
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			for target := range jobs {
				results <- ping.probeTarget(probeCtx, target, options)
			}
		}()
	}
	go feedPingTargets(probeCtx, jobs, targets)
	go func() {
		group.Wait()
		close(results)
	}()
	collected := make([]pingmodel.Result, 0, len(targets))
	var observeErr error
	for result := range results {
		collected = append(collected, result)
		if observe != nil && observeErr == nil {
			if err := observe(result); err != nil {
				observeErr = err
				cancel()
			}
		}
	}
	if observeErr != nil {
		return nil, observeErr
	}
	return collected, nil
}

func feedPingTargets(ctx context.Context, jobs chan<- pingmodel.Target, targets []pingmodel.Target) {
	defer close(jobs)
	for _, target := range targets {
		select {
		case <-ctx.Done():
			return
		case jobs <- target:
		}
	}
}

func (ping *OpenAI) probeTarget(ctx context.Context, target pingmodel.Target, options pingmodel.Options) pingmodel.Result {
	started := time.Now()
	processResult, err := ping.process.PingOpenAI(ctx, target, options)
	if err != nil {
		return processErrorResult(target.Name, options.Model, options.Effort, err, time.Since(started).Milliseconds())
	}
	status, detail, effectiveModel := classifyProcessResult(processResult)
	return newResult(target.Name, options.Model, options.Effort, status, detail, processResult.FirstResponseMS, processResult.LatencyMS, effectiveModel)
}

func classifyProcessResult(result pingmodel.ProcessResult) (pingmodel.Status, string, string) {
	if result.CleanupFailed {
		status := pingmodel.ProcessFailed
		return status, decorateProcessFailure(status, "OpenAI application ping cleanup failed", result), ""
	}
	if result.Cancelled {
		status := pingmodel.Cancelled
		return status, decorateProcessFailure(status, statusDetail(status), result), ""
	}
	if result.TimedOut {
		status := pingmodel.Timeout
		return status, decorateProcessFailure(status, statusDetail(status), result), ""
	}
	status, detail, effectiveModel := validateJSONL(result.Stdout)
	if status == pingmodel.Pass && result.ExitCode == 0 {
		return status, detail, effectiveModel
	}
	if status == pingmodel.ProtocolFailed || status == pingmodel.UnexpectedResponse {
		status, detail = classifyEmptyProtocolFailure(status, detail, result)
	}
	return status, decorateProcessFailure(status, detail, result), effectiveModel
}

func classifyEmptyProtocolFailure(status pingmodel.Status, detail string, result pingmodel.ProcessResult) (pingmodel.Status, string) {
	if strings.TrimSpace(string(result.Stdout)) != "" {
		return status, detail
	}
	failure := classifyFailure(string(result.Stderr))
	if failure != pingmodel.ProcessFailed {
		return failure, statusDetail(failure)
	}
	if result.ExitCode != 0 {
		return pingmodel.ProcessFailed, statusDetail(pingmodel.ProcessFailed)
	}
	return status, detail
}

func newResult(profile, model, effort string, status pingmodel.Status, detail string, first *int64, latency int64, effective string) pingmodel.Result {
	completion := latency
	return pingmodel.Result{
		Profile: profile, Status: status, Model: model, RequestedModel: model, Effort: effort, RequestedEffort: effort, EffectiveModel: effective,
		CredentialValidation: credentialValidation(status), FirstResponseLatencyMS: first,
		CompletionLatencyMS: &completion, LatencyMS: &completion, Detail: detail,
	}
}

func buildReport(results []pingmodel.Result, discovered int, elapsed time.Duration) pingmodel.Report {
	summary := pingmodel.Summary{ProfilesDiscovered: discovered, ProfilesTested: len(results), DurationMS: elapsed.Milliseconds()}
	for _, result := range results {
		accumulateSummary(&summary, result.Status)
	}
	summary.PoolUsable = summary.Healthy > 0
	status := "failed"
	if summary.Healthy == len(results) && len(results) > 0 {
		status = "ok"
	}
	requestedModel, requestedEffort, effective := "", "", ""
	if len(results) > 0 {
		requestedModel = results[0].Model
		requestedEffort = results[0].Effort
		effective = results[0].EffectiveModel
	}
	return pingmodel.Report{
		Provider: "openai", Status: status, Model: requestedModel, RequestedModel: requestedModel,
		Effort: requestedEffort, RequestedEffort: requestedEffort, EffectiveModel: effective, LatencyMS: elapsed.Milliseconds(),
		Detail:   fmt.Sprintf("%d/%d profiles healthy", summary.Healthy, len(results)),
		Profiles: results, Summary: summary,
	}
}

func accumulateSummary(summary *pingmodel.Summary, status pingmodel.Status) {
	switch status {
	case pingmodel.Pass:
		summary.Healthy++
	case pingmodel.QuotaExhausted:
		summary.Exhausted++
	case pingmodel.AuthFailed:
		summary.AuthFailures++
	default:
		if temporary(status) {
			summary.TemporaryFailures++
		} else {
			summary.OtherFailures++
		}
	}
}

func ValidateOptions(options pingmodel.Options) error { return validateOptions(options) }

func validateOptions(options pingmodel.Options) error {
	for name, value := range map[string]string{"--profile": options.Profile, "--model": options.Model, "--effort": options.Effort} {
		if value != "" && (strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00")) {
			return fmt.Errorf("%s must be nonempty and contain no control characters", name)
		}
	}
	if options.BaseURL == "" {
		return nil
	}
	parsed, err := url.Parse(options.BaseURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("ping upstream base URL must be a credential-free http(s) URL without query data")
	}
	return nil
}

func Failed(report pingmodel.Report) bool {
	return len(report.Profiles) == 0 || report.Summary.Healthy != len(report.Profiles)
}

func HumanStatus(status pingmodel.Status) string { return humanStatus(status) }
