package deepseek

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func deepSeekToolArgumentError(name, arguments string, cause error) error {
	if !deepSeekBufferedFragmentValid(arguments) {
		if reason, offset := deepSeekBufferedSerdeFailure(arguments); reason != "" {
			return deepSeekToolArgumentDiagnostic(name, arguments, reason, offset)
		}
	}
	// shortcut: other diagnostic classes retain their existing errors; extend with pinned serde regressions.
	var value json.RawMessage
	var syntax *json.SyntaxError
	if err := json.Unmarshal([]byte(arguments), &value); errors.As(err, &syntax) {
		diagnostic := ""
		switch {
		case strings.Contains(syntax.Error(), "looking for beginning of object key string"):
			diagnostic = "key must be a string"
			if arguments[syntax.Offset-1] == '}' {
				diagnostic = "trailing comma"
			}
		case strings.Contains(syntax.Error(), "after object key:value pair"):
			diagnostic = "expected `,` or `}`"
		case strings.Contains(syntax.Error(), "after object key"):
			diagnostic = "expected `:`"
		case strings.Contains(syntax.Error(), "after array element"):
			diagnostic = "expected `,` or `]`"
		case strings.Contains(syntax.Error(), "after top-level value"):
			diagnostic = "trailing characters"
		case strings.Contains(syntax.Error(), "looking for beginning of value") &&
			arguments[syntax.Offset-1] == ']' &&
			strings.HasSuffix(strings.TrimSpace(arguments[:syntax.Offset-1]), ","):
			diagnostic = "trailing comma"
		}
		if diagnostic != "" {
			return deepSeekToolArgumentDiagnostic(name, arguments, diagnostic, int(syntax.Offset))
		}
	}
	return deepSeekMalformedArgumentError(name, cause)
}

func deepSeekToolArgumentDiagnostic(name, arguments, reason string, offset int) error {
	// Serde columns count consumed UTF-8 bytes, including the unexpected byte.
	prefix := arguments[:offset]
	line := strings.Count(prefix, "\n") + 1
	column := len(prefix) - strings.LastIndex(prefix, "\n") - 1
	return deepSeekMalformedArgumentError(name, fmt.Errorf("%s at line %d column %d", reason, line, column))
}

func deepSeekMalformedArgumentError(name string, cause error) error {
	return fmt.Errorf("DeepSeek returned malformed JSON arguments for tool call `%s`: %w", name, cause)
}
