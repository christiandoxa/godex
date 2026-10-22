package deepseek

import "unicode/utf8"

// Buffered ordinary arguments use the tagged bounded JSON view, which keeps
// primitive/string contents opaque. Structured search has its own serde boundary.
func deepSeekBufferedFragmentValid(raw string) bool {
	if !utf8.ValidString(raw) {
		return false
	}
	end := deepSeekBufferedFragmentEnd(raw, 0, 0)
	return end >= 0 && deepSeekBufferedSkipSpace(raw, end) == len(raw)
}

func deepSeekBufferedFragmentEnd(raw string, start, depth int) int {
	if depth > 256 {
		return -1
	}
	index := deepSeekBufferedSkipSpace(raw, start)
	if index == len(raw) {
		return -1
	}
	switch raw[index] {
	case '"':
		return deepSeekBufferedStringEnd(raw, index)
	case '[', '{':
		return deepSeekBufferedContainerEnd(raw, index, depth)
	}
	start = index
	for index < len(raw) {
		switch raw[index] {
		case '\t', '\n', '\r', ' ', ',', ']', '}':
			if index == start {
				return -1
			}
			return index
		}
		index++
	}
	return index
}

func deepSeekBufferedContainerEnd(raw string, index, depth int) int {
	object, closing := raw[index] == '{', byte(']')
	if object {
		closing = '}'
	}
	index = deepSeekBufferedSkipSpace(raw, index+1)
	if index < len(raw) && raw[index] == closing {
		return index + 1
	}
	for index < len(raw) {
		if object {
			keyEnd := deepSeekBufferedStringEnd(raw, index)
			if keyEnd < 0 {
				return -1
			}
			index = deepSeekBufferedSkipSpace(raw, keyEnd)
			if index == len(raw) || raw[index] != ':' {
				return -1
			}
			index++
		}
		end := deepSeekBufferedFragmentEnd(raw, index, depth+1)
		if end < 0 {
			return -1
		}
		index = deepSeekBufferedSkipSpace(raw, end)
		if index < len(raw) && raw[index] == ',' {
			index = deepSeekBufferedSkipSpace(raw, index+1)
			continue
		}
		if index < len(raw) && raw[index] == closing {
			return index + 1
		}
		return -1
	}
	return -1
}

func deepSeekBufferedStringEnd(raw string, start int) int {
	if start >= len(raw) || raw[start] != '"' {
		return -1
	}
	for index := start + 1; index < len(raw); index++ {
		switch {
		case raw[index] == '\\':
			index++
		case raw[index] == '"':
			return index + 1
		case raw[index] < 32:
			return -1
		}
	}
	return -1
}

func deepSeekBufferedSkipSpace(raw string, index int) int {
	for index < len(raw) && (raw[index] == ' ' || raw[index] == '\t' || raw[index] == '\n' || raw[index] == '\r') {
		index++
	}
	return index
}
