package superexpose

import "strings"

// sanitizeOutputText follows the tagged transcript viewer: remove terminal
// control sequences before replacing the remaining unsafe controls.
func sanitizeOutputText(value string) string {
	value = stripTranscriptANSI043712(value)
	var builder strings.Builder
	for _, current := range value {
		if current < 0x20 && current != '\n' && current != '\t' ||
			(current >= 0x7f && current <= 0x9f) {
			builder.WriteByte(' ')
		} else {
			builder.WriteRune(current)
		}
	}
	text := builder.String()
	if len(text) <= outputReadMaxTextBytes {
		return text
	}
	const marker = " …[text_truncated]"
	limit := outputReadMaxTextBytes - len(marker)
	for limit > 0 && !utf8Boundary(text, limit) {
		limit--
	}
	return text[:limit] + marker
}

// stripTranscriptANSI043712 removes CSI and OSC sequences byte-for-byte while
// consuming one full UTF-8 scalar for other ESC forms.
func stripTranscriptANSI043712(input string) string {
	var output strings.Builder
	output.Grow(len(input))
	for cursor := 0; cursor < len(input); {
		value := input[cursor]
		cursor++
		if value == 0x1b {
			if cursor >= len(input) {
				break
			}
			escapeType := input[cursor]
			cursor++
			switch escapeType {
			case '[':
				for cursor < len(input) {
					part := input[cursor]
					cursor++
					if part >= '@' && part <= '~' {
						break
					}
				}
			case ']':
				var previous byte
				for cursor < len(input) {
					part := input[cursor]
					cursor++
					if part == 0x07 || previous == 0x1b && part == '\\' {
						break
					}
					previous = part
				}
			default:
				cursor += utf8ContinuationBytes043712(escapeType)
				if cursor > len(input) {
					break
				}
			}
			continue
		}
		if value == '\r' {
			if cursor < len(input) && input[cursor] == '\n' {
				continue
			}
			value = '\n'
		}
		output.WriteByte(value)
	}
	return output.String()
}

func utf8ContinuationBytes043712(first byte) int {
	switch {
	case first >= 0xc2 && first <= 0xdf:
		return 1
	case first >= 0xe0 && first <= 0xef:
		return 2
	case first >= 0xf0 && first <= 0xf4:
		return 3
	default:
		return 0
	}
}
