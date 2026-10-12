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
	for _, rawLine := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(rawLine, "\r"))
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if smartContextLineHasError(lower) {
			counts.errors++
		}
		locations := len(smartContextFileLocationPattern.FindAllString(line, -1))
		if locations == 0 && (strings.Contains(lower, "file \\") || strings.Contains(lower, "file '")) {
			locations = 1
		}
		counts.fileLocations += locations
		if strings.Contains(line, "@@ -") || (strings.HasPrefix(line, "@@ ") && strings.Contains(line[3:], "@@")) {
			counts.diffHunks++
		}
		if smartContextLineHasTestFailure(lower) {
			counts.testFailures++
		}
		if strings.Contains(lower, "exit code ") || strings.Contains(lower, "exit status ") || strings.Contains(lower, "exit_status") {
			counts.exitCodes++
		}
		if strings.Contains(lower, "stack backtrace:") || strings.Contains(lower, "traceback (most recent call last):") ||
			strings.Contains(lower, "stack trace:") || strings.Contains(lower, "backtrace:") {
			counts.stackMarkers++
		}
		counts.rustDiagnostics += smartContextRustDiagnosticCount(lower)
	}
	return counts
}

func smartContextLineHasError(lower string) bool {
	for _, marker := range []string{
		"error[", "error:", "assertionerror:", "runtimeerror:", "syntaxerror:",
		"typeerror:", "valueerror:", "exception:",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func smartContextLineHasTestFailure(lower string) bool {
	return strings.Contains(lower, " failed") || strings.Contains(lower, "... failed") ||
		strings.Contains(lower, "failures=") || strings.HasPrefix(lower, "fail ") ||
		strings.HasPrefix(lower, "failed ")
}

func smartContextRustDiagnosticCount(lower string) int {
	count := 0
	for _, marker := range []string{
		"error:", "error[", "warning:", "warning[", "--> ", "::: ",
		"= note:", "= help:", "help:", "note:", "clippy::",
	} {
		count += strings.Count(lower, marker)
	}
	return count
}
