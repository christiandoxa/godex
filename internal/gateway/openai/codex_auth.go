package openai

import "github.com/christiandoxa/godex/internal/gateway/codex"

func readAccessToken(path string) (string, error) {
	return codex.ReadAccessToken(path)
}
