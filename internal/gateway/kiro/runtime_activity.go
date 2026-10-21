package kiro

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	kiroActivityNameMaxBytes = 160
	kiroActivityKindMaxBytes = 48
)

var kiroActivitySensitiveTerms = []string{
	"authorization", "bearer", "api_key", "apikey", "password",
	"sk-", "sk_", "secret", "token", "credential",
}

func kiroActivityItem(title, status, kind string, initial, detailsOmitted bool) map[string]any {
	safeTitle := kiroActivityField(title, kiroActivityNameMaxBytes)
	safeKind := kiroActivityField(kind, kiroActivityKindMaxBytes)
	name := safeTitle
	if name == "" {
		name = safeKind
	}
	if name == "" {
		name = "Kiro internal activity"
	}
	normalizedStatus := kiroActivityStatus(status)
	phase := kiroActivityPhase(normalizedStatus, initial)
	item := map[string]any{
		"type":                  "kiro_internal_activity",
		"name":                  name,
		kiroFieldStatus:         normalizedStatus,
		kiroFieldPhase:          phase,
		"kind":                  nil,
		kiroFieldDetailsOmitted: detailsOmitted,
	}
	if safeKind != "" {
		item["kind"] = safeKind
	}
	return item
}

func kiroActivityText(item map[string]any) string {
	name, _ := item["name"].(string)
	status, _ := item[kiroFieldStatus].(string)
	phase, _ := item[kiroFieldPhase].(string)
	var output strings.Builder
	output.WriteString("[Kiro activity: ")
	output.WriteString(name)
	output.WriteString("; status=")
	output.WriteString(status)
	output.WriteString("; phase=")
	output.WriteString(phase)
	if kind, ok := item["kind"].(string); ok && kind != "" {
		output.WriteString("; kind=")
		output.WriteString(kind)
	}
	if details, _ := item[kiroFieldDetailsOmitted].(bool); details {
		output.WriteString("; details=omitted")
	}
	output.WriteString("]\n")
	return output.String()
}

func kiroActivityField(value string, maximum int) string {
	value = strings.TrimSpace(value)
	if value == "" || kiroActivityUnsafe(value) {
		return ""
	}
	value = strings.Join(strings.FieldsFunc(value, unicode.IsSpace), " ")
	return truncateActivityUTF8(value, maximum)
}

// Kiro's compiled activity kernel folds only ASCII capital letters.
// Keep all non-ASCII UTF-8 bytes byte-for-byte rather than normalizing or
// Unicode-casefolding potentially meaningful activity names.
func kiroASCIIOnlyLower04384(value string) string {
	buffer := []byte(value)
	for i := range buffer {
		if buffer[i] >= 'A' && buffer[i] <= 'Z' {
			buffer[i] += 'a' - 'A'
		}
	}
	return string(buffer)
}

func kiroActivityUnsafe(value string) bool {
	// Tagged kiro.mojo compares forbidden labels after ASCII-only byte
	// folding. Unicode lowercase could collapse Kelvin K into ASCII k or
	// dotted İ into i, incorrectly hiding a distinct non-ASCII label.
	lower := kiroASCIIOnlyLower04384(value)
	for _, term := range kiroActivitySensitiveTerms {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return strings.ContainsAny(value, "/\\@")
}

func kiroActivityStatus(value string) string {
	// Mojo rich_view_matches_literal(..., True) folds A-Z bytes only.
	// An uppercase Unicode lookalike is not a recognized ASCII status.
	lower := kiroASCIIOnlyLower04384(strings.TrimSpace(value))
	switch lower {
	case "pending", "in_progress", "running", kiroStatusCompleted,
		kiroStatusFailed, "error", kiroStatusCancelled, kiroStatusTruncated:
		return lower
	default:
		return "unknown"
	}
}

func kiroActivityPhase(status string, initial bool) string {
	switch status {
	case kiroStatusCompleted:
		return kiroStatusCompleted
	case kiroStatusFailed, "error":
		return kiroStatusFailed
	case kiroStatusCancelled:
		return kiroStatusCancelled
	case kiroStatusTruncated:
		return kiroStatusTruncated
	default:
		if initial {
			return "started"
		}
		return "updated"
	}
}

func truncateActivityUTF8(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	end := maximum
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end]
}

func kiroTruncatedActivity() map[string]any {
	return map[string]any{
		"type":                  "kiro_internal_activity",
		"name":                  "Kiro internal activity",
		kiroFieldStatus:         kiroStatusTruncated,
		kiroFieldPhase:          kiroStatusTruncated,
		"kind":                  nil,
		kiroFieldDetailsOmitted: true,
	}
}
