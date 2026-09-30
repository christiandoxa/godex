package codex

import (
	"errors"
	"io"
	"net/url"
)

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
