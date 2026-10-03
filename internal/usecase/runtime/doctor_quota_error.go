package runtime

import (
	"strings"

	redacthelper "github.com/christiandoxa/godex/internal/helper/redact"
)

func doctorQuotaErrorSummary(err error) string {
	if err == nil {
		return ""
	}
	redacted := redacthelper.Secrets(err.Error())
	line := "-"
	for _, candidate := range strings.Split(redacted, "\n") {
		if trimmed := strings.TrimSpace(candidate); trimmed != "" {
			line = trimmed
			break
		}
	}
	return "Error (" + line + ")"
}
