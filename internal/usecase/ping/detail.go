package ping

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	pingmodel "github.com/christiandoxa/godex/internal/model/ping"
)

const pingErrorDetailMaxBytes = 4096

var pingSecretPatterns = []struct {
	pattern *regexp.Regexp
	replace string
}{
	{regexp.MustCompile(`(?i)(authorization\s*:\s*bearer\s+)[^\s]+`), `${1}<redacted>`},
	{regexp.MustCompile(`(?i)(["']?(?:access_token|refresh_token|id_token|api_key|openai_api_key|anthropic_api_key|gemini_api_key|google_api_key|github_copilot_api_key)["']?\s*[:=]\s*["']?)[^"',\s}]+`), `${1}<redacted>`},
	{regexp.MustCompile(`(?i)(https?://)[^/@\s]+@`), `${1}<redacted>@`},
	{regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{8,}`), `<redacted>`},
	{regexp.MustCompile(`\bgh[opsu]_[A-Za-z0-9]{20,}`), `<redacted>`},
	{regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`), `<redacted>`},
}

func processErrorResult(profile, model string, err error, latency int64) pingmodel.Result {
	status := classifyFailure(err.Error())
	if status == pingmodel.ProcessFailed && strings.Contains(strings.ToLower(err.Error()), "failed to start") {
		status = pingmodel.SpawnFailed
	}
	detail := appendPingDetail(statusDetail(status), boundedPingDetail(err.Error()))
	return newResult(profile, model, status, detail, nil, latency, "")
}

func decorateProcessFailure(status pingmodel.Status, detail string, result pingmodel.ProcessResult) string {
	if status == pingmodel.Pass {
		return detail
	}
	if result.ExitCode != 0 {
		detail = appendPingDetail(detail, fmt.Sprintf("exit code %d", result.ExitCode))
	}
	return appendPingDetail(detail, boundedPingDetail(string(result.Stderr)))
}

func appendPingDetail(base, extra string) string {
	extra = strings.TrimSpace(extra)
	if extra == "" || extra == base {
		return base
	}
	return base + ": " + extra
}

func boundedPingDetail(value string) string {
	value = strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ").Replace(value))
	for _, secret := range pingSecretPatterns {
		value = secret.pattern.ReplaceAllString(value, secret.replace)
	}
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= pingErrorDetailMaxBytes {
		return value
	}
	end := pingErrorDetailMaxBytes
	for end > 0 && end < len(value) && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end]
}
