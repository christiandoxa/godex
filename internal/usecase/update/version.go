package update

import (
	"errors"
	"strconv"
	"strings"
)

type semanticVersion struct {
	major, minor, patch uint64
	prerelease          []string
	build               []string
}

func parseVersion(value string) (semanticVersion, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "v") {
		value = value[1:]
	}
	if value == "" {
		return semanticVersion{}, errors.New("empty version")
	}

	coreAndPre, buildText, hasBuild := strings.Cut(value, "+")
	if hasBuild {
		if buildText == "" || strings.Contains(buildText, "+") {
			return semanticVersion{}, errors.New("invalid build metadata")
		}
	}

	parts := strings.SplitN(coreAndPre, "-", 2)
	core := strings.Split(parts[0], ".")
	if len(core) != 3 {
		return semanticVersion{}, errors.New("version must have major.minor.patch")
	}
	values := make([]uint64, 3)
	for index, part := range core {
		if !validNumericIdentifier(part) {
			return semanticVersion{}, errors.New("invalid numeric version component")
		}
		parsed, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return semanticVersion{}, errors.New("version component is out of range")
		}
		values[index] = parsed
	}
	version := semanticVersion{major: values[0], minor: values[1], patch: values[2]}

	if len(parts) == 2 {
		if parts[1] == "" {
			return semanticVersion{}, errors.New("empty prerelease")
		}
		for _, identifier := range strings.Split(parts[1], ".") {
			if !validPrereleaseIdentifier(identifier) {
				return semanticVersion{}, errors.New("invalid prerelease identifier")
			}
			if numericIdentifier(identifier) && len(identifier) > 1 && identifier[0] == '0' {
				return semanticVersion{}, errors.New("numeric prerelease identifier has a leading zero")
			}
			version.prerelease = append(version.prerelease, identifier)
		}
	}

	if hasBuild {
		for _, identifier := range strings.Split(buildText, ".") {
			if !validPrereleaseIdentifier(identifier) {
				return semanticVersion{}, errors.New("invalid build metadata identifier")
			}
			version.build = append(version.build, identifier)
		}
	}
	return version, nil
}

func compareVersions(left, right semanticVersion) int {
	for _, pair := range [][2]uint64{{left.major, right.major}, {left.minor, right.minor}, {left.patch, right.patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	return comparePrerelease(left.prerelease, right.prerelease)
}

func compareVersionsTotal(left, right semanticVersion) int {
	if comparison := compareVersions(left, right); comparison != 0 {
		return comparison
	}
	return compareBuildMetadata(left.build, right.build)
}

func compareBuildMetadata(left, right []string) int {
	if len(left) == 0 && len(right) == 0 {
		return 0
	}
	if len(left) == 0 {
		return -1
	}
	if len(right) == 0 {
		return 1
	}
	for index := 0; index < min(len(left), len(right)); index++ {
		if comparison := compareBuildIdentifier(left[index], right[index]); comparison != 0 {
			return comparison
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

func compareBuildIdentifier(left, right string) int {
	leftNumeric, rightNumeric := numericIdentifier(left), numericIdentifier(right)
	if leftNumeric && !rightNumeric {
		return -1
	}
	if !leftNumeric && rightNumeric {
		return 1
	}
	if !leftNumeric {
		return strings.Compare(left, right)
	}

	leftSignificant := strings.TrimLeft(left, "0")
	rightSignificant := strings.TrimLeft(right, "0")
	if len(leftSignificant) < len(rightSignificant) {
		return -1
	}
	if len(leftSignificant) > len(rightSignificant) {
		return 1
	}
	if comparison := strings.Compare(leftSignificant, rightSignificant); comparison != 0 {
		return comparison
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

func comparePrerelease(left, right []string) int {
	if len(left) == 0 && len(right) == 0 {
		return 0
	}
	if len(left) == 0 {
		return 1
	}
	if len(right) == 0 {
		return -1
	}
	for index := 0; index < min(len(left), len(right)); index++ {
		if comparison := comparePrereleaseIdentifier(left[index], right[index]); comparison != 0 {
			return comparison
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

func comparePrereleaseIdentifier(left, right string) int {
	leftNumeric, rightNumeric := numericIdentifier(left), numericIdentifier(right)
	if leftNumeric && rightNumeric {
		leftValue, _ := strconv.ParseUint(left, 10, 64)
		rightValue, _ := strconv.ParseUint(right, 10, 64)
		if leftValue < rightValue {
			return -1
		}
		if leftValue > rightValue {
			return 1
		}
		return 0
	}
	if leftNumeric {
		return -1
	}
	if rightNumeric {
		return 1
	}
	return strings.Compare(left, right)
}

func validNumericIdentifier(value string) bool {
	if value == "" || (len(value) > 1 && value[0] == '0') {
		return false
	}
	return numericIdentifier(value)
}

func numericIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, current := range value {
		if current < '0' || current > '9' {
			return false
		}
	}
	return true
}

func validPrereleaseIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, current := range value {
		if (current >= 'a' && current <= 'z') || (current >= 'A' && current <= 'Z') ||
			(current >= '0' && current <= '9') || current == '-' {
			continue
		}
		return false
	}
	return true
}
