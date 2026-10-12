package proxy

import (
	"regexp"
	"strings"
)

var smartContextFileLocationPattern = regexp.MustCompile("[A-Za-z0-9_./\\\\-]+:[0-9]+(?::[0-9]+)?")

type smartContextSignalCounts struct {
	errors          int
	fileLocations   int
	diffHunks       int
	testFailures    int
	exitCodes       int
	stackMarkers    int
	rustDiagnostics int
}

func smartContextCriticalSignalsPreserved(before, after []byte) bool {
	beforeCounts := smartContextCriticalSignalCounts(string(before))
	afterCounts := smartContextCriticalSignalCounts(string(after))
	return afterCounts.errors >= beforeCounts.errors &&
		afterCounts.fileLocations >= beforeCounts.fileLocations &&
		afterCounts.diffHunks >= beforeCounts.diffHunks &&
		afterCounts.testFailures >= beforeCounts.testFailures &&
		afterCounts.exitCodes >= beforeCounts.exitCodes &&
		afterCounts.stackMarkers >= beforeCounts.stackMarkers &&
		afterCounts.rustDiagnostics >= beforeCounts.rustDiagnostics
}

func smartContextCriticalSignalCounts(text string) smartContextSignalCounts {
	counts := smartContextSignalCounts{}
	for _, rawLine := range strings.Split(strings.TrimRight(text, "\r\n"), "\n") {
		line := smartContextSignalLine(rawLine)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if smartContextErrorLine(lower) {
			counts.errors++
		}
		counts.fileLocations += len(smartContextFileLocationPattern.FindAllStringIndex(line, -1))
		if strings.Contains(lower, `file "`) || strings.Contains(lower, "file '") {
			counts.fileLocations++
		}
		if strings.HasPrefix(line, "@@ ") && strings.Contains(line[3:], "@@") {
			counts.diffHunks++
		}
		if smartContextTestFailureLine(line) {
			counts.testFailures++
		}
		if strings.Contains(lower, "exit status") || strings.Contains(lower, "exit code") ||
			strings.Contains(lower, "exit_status") {
			counts.exitCodes++
		}
		if smartContextStackLine(line) {
			counts.stackMarkers++
		}
		if smartContextDiagnosticLine(line) {
			counts.rustDiagnostics++
		}
	}
	return counts
}

func smartContextSignalLine(raw string) string {
	line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
	if len(line) < 3 || line[0] != 'L' {
		return line
	}
	index := 1
	for index < len(line) && line[index] >= '0' && line[index] <= '9' {
		index++
	}
	if index > 1 && index+1 < len(line) && line[index] == ':' && line[index+1] == ' ' {
		return strings.TrimSpace(line[index+2:])
	}
	return line
}

func smartContextErrorLine(lower string) bool {
	if smartContextZeroSummary(lower, "error:") || smartContextZeroSummary(lower, "errors:") {
		return false
	}
	for _, prefix := range []string{
		"error:", "error[", "error ", "error\t", "fatal:", "panic:",
		"npm err!", "npm error", "pnpm error", "yarn error", "bun error",
		"failed ", "fail ", "e   ", "assertionerror:", "runtimeerror:",
		"syntaxerror:", "typeerror:", "valueerror:", "exception:",
	} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return strings.Contains(lower, "panicked at") || strings.Contains(lower, `"error"`) ||
		strings.Contains(lower, "status=error") || strings.Contains(lower, "level=error") ||
		strings.Contains(lower, "exception")
}

func smartContextZeroSummary(lower, prefix string) bool {
	if !strings.HasPrefix(lower, prefix) {
		return false
	}
	index := len(prefix)
	for index < len(lower) && strings.ContainsRune(" \t:=(", rune(lower[index])) {
		index++
	}
	if index == len(lower) || lower[index] != '0' {
		return false
	}
	for index < len(lower) && lower[index] == '0' {
		index++
	}
	for index < len(lower) && strings.ContainsRune(" \t)", rune(lower[index])) {
		index++
	}
	return index == len(lower)
}

func smartContextTestFailureLine(line string) bool {
	return strings.HasPrefix(line, "test ") && strings.Contains(line, " ... FAILED") ||
		strings.HasPrefix(line, "---- ") && strings.Contains(line, " ----") ||
		strings.HasPrefix(line, "FAIL ") || strings.HasPrefix(line, "FAILED ") ||
		strings.HasPrefix(line, "test result: FAILED") || strings.HasPrefix(line, "failures:") ||
		strings.Contains(line, " ... FAILED")
}

func smartContextStackLine(line string) bool {
	for _, prefix := range []string{
		"stack backtrace:", "Traceback (most recent call last):", "Stack trace:",
		"stack trace:", "Backtrace:", "Caused by:",
	} {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func smartContextDiagnosticLine(line string) bool {
	for _, prefix := range []string{
		"error:", "error[", "warning:", "warning[", "--> ", "::: ",
		"= note:", "= help:", "help:", "note:",
	} {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return strings.Contains(line, "clippy::")
}
