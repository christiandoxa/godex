package ping

import (
	"fmt"
	"strings"
	"unicode/utf8"

	redacthelper "github.com/christiandoxa/godex/internal/helper/redact"
	pingmodel "github.com/christiandoxa/godex/internal/model/ping"
)

const pingErrorDetailMaxBytes = 4096

func processErrorResult(profile, model, effort string, err error, latency int64) pingmodel.Result {
	status := classifyFailure(err.Error())
	if status == pingmodel.ProcessFailed && strings.Contains(strings.ToLower(err.Error()), "failed to start") {
		status = pingmodel.SpawnFailed
	}
	detail := appendPingDetail(statusDetail(status), boundedPingDetail(err.Error()))
	return newResult(profile, model, effort, status, detail, nil, latency, "")
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
	value = redacthelper.Secrets(value)
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
