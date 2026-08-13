package codex

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
)

// RuntimeConfig contains only profile-local Codex URL overrides. Empty values
// leave the generated profile config untouched.
type RuntimeConfig struct {
	ChatGPTBaseURL string
	OpenAIBaseURL  string
}

func (config RuntimeConfig) arguments() ([]string, error) {
	arguments := make([]string, 0, 4)
	for _, value := range []struct {
		key string
		url string
	}{
		{key: "chatgpt_base_url", url: config.ChatGPTBaseURL},
		{key: "openai_base_url", url: config.OpenAIBaseURL},
	} {
		if strings.TrimSpace(value.url) == "" {
			continue
		}
		if err := validateRuntimeURL(value.url); err != nil {
			return nil, fmt.Errorf("invalid Codex runtime URL for %s: %w", value.key, err)
		}
		arguments = append(arguments, "-c", value.key+"="+strconv.Quote(value.url))
	}
	return arguments, nil
}

func validateRuntimeURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("must be an http(s) URL without credentials or query data")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("must use http or https")
	}
	return nil
}

type Terminal struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}
