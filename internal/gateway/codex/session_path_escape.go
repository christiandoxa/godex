package codex

import "strings"

func sessionPathEscapeWidth(raw string) int {
	minimum := 0
	run := 0
	for index := 0; index <= len(raw); index++ {
		if index < len(raw) && raw[index] == '\\' {
			run++
			continue
		}
		if run > 0 && (minimum == 0 || run < minimum) {
			minimum = run
		}
		run = 0
	}
	if minimum == 0 {
		return 1
	}
	return minimum
}

func decodeSessionPathEscaped(raw string, width int) string {
	if width <= 1 {
		return raw
	}
	var output strings.Builder
	output.Grow(len(raw))
	backslashes := 0
	flush := func() {
		for count := (backslashes + width - 1) / width; count > 0; count-- {
			output.WriteByte('\\')
		}
		backslashes = 0
	}
	for _, character := range raw {
		if character == '\\' {
			backslashes++
			continue
		}
		flush()
		output.WriteRune(character)
	}
	flush()
	return output.String()
}
