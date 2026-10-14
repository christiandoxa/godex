package gemini

// The 0.437.11 Mojo Gemini request kernel uses its own Unicode whitespace
// classifier for thought-signature presence. Unlike Go strings.TrimSpace, it
// also treats U+001C–U+001F as space. This is deliberately an exact list:
// unlisted format marks can be significant pieces of a signed token.
func geminiSignatureHasMojoNonSpace043711(text string) bool {
	for _, point := range text {
		switch {
		case point >= '\u0009' && point <= '\u000d':
			continue
		case point >= '\u001c' && point <= '\u001f':
			continue
		case point >= '\u2000' && point <= '\u200a':
			continue
		case point == ' ', point == '\u0085', point == '\u00a0', point == '\u1680',
			point == '\u2028', point == '\u2029', point == '\u202f',
			point == '\u205f', point == '\u3000':
			continue
		default:
			return true
		}
	}
	return false
}
