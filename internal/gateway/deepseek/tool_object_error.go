package deepseek

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func deepSeekToolArgumentError(name, arguments string, cause error) error {
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
		}
		if diagnostic != "" {
			// Serde columns count UTF-8 bytes, including the unexpected byte.
			prefix := arguments[:syntax.Offset]
			line := strings.Count(prefix, "\n") + 1
			column := len(prefix) - strings.LastIndex(prefix, "\n") - 1
			cause = fmt.Errorf("%s at line %d column %d", diagnostic, line, column)
		}
	}
	return fmt.Errorf("DeepSeek returned malformed JSON arguments for tool call `%s`: %w", name, cause)
}
