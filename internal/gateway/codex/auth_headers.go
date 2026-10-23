package codex

import (
	"context"
	"os"
	"strings"
	"unicode/utf8"

	authmodel "github.com/christiandoxa/godex/internal/model/auth"
)

func (*CodexProcess) ReadQuotaHeaders(ctx context.Context, home string) (authmodel.RequestHeaders, error) {
	config, err := readCodexConfig(ctx, home)
	if err != nil {
		return authmodel.RequestHeaders{}, err
	}
	headers := authmodel.RequestHeaders{Originator: codexAuthOriginator()}
	value, _ := config["enforce_residency"].(string)
	if strings.ToLower(strings.TrimSpace(value)) == "us" {
		headers.Residency = "us"
	}
	return headers, nil
}

func codexAuthOriginator() string {
	const fallback = "codex_cli_rs"
	value, found := os.LookupEnv("CODEX_INTERNAL_ORIGINATOR_OVERRIDE")
	if !found || !utf8.ValidString(value) {
		return fallback
	}
	for _, b := range []byte(value) {
		if (b < 0x20 && b != '\t') || b == 0x7f {
			return fallback
		}
	}
	return value
}
