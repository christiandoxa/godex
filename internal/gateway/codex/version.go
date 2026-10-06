package codex

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

const (
	MinimumVersion = "0.153.2"
	AuditedVersion = "0.160.1"
)

var codexVersionPattern = regexp.MustCompile(`(?:^|[^0-9])([0-9]+)\.([0-9]+)\.([0-9]+)(?:[^0-9]|$)`)

type semanticVersion struct {
	major int
	minor int
	patch int
}

func requireSupportedVersion(output string) error {
	current, err := parseCodexVersion(output)
	if err != nil {
		return err
	}
	minimum, err := parseCodexVersion(MinimumVersion)
	if err != nil {
		return errors.New("invalid built-in Codex minimum version")
	}
	if current.lessThan(minimum) {
		return fmt.Errorf("official Codex CLI %s or newer is required; found %s", MinimumVersion, current.String())
	}
	return nil
}

func parseCodexVersion(output string) (semanticVersion, error) {
	match := codexVersionPattern.FindStringSubmatch(output)
	if len(match) != 4 {
		return semanticVersion{}, fmt.Errorf("could not parse official Codex CLI version from %q", output)
	}
	parts := [3]int{}
	for index := range parts {
		value, err := strconv.Atoi(match[index+1])
		if err != nil {
			return semanticVersion{}, errors.New("Codex CLI version contains an invalid numeric component")
		}
		parts[index] = value
	}
	return semanticVersion{major: parts[0], minor: parts[1], patch: parts[2]}, nil
}

func (version semanticVersion) lessThan(other semanticVersion) bool {
	if version.major != other.major {
		return version.major < other.major
	}
	if version.minor != other.minor {
		return version.minor < other.minor
	}
	return version.patch < other.patch
}

func (version semanticVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", version.major, version.minor, version.patch)
}
