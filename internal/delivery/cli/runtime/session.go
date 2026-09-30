package runtime

import (
	"strings"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
)

// Only explicit UUIDs/prefixes are cross-profile. Native names and pickers stay local.
func sessionArgument(arguments []string) (int, []string) {
	args := append([]string(nil), arguments...)
	if len(args) == 1 && sessionentity.ValidID(args[0]) {
		return 1, []string{"resume", args[0]}
	}
	start := -1
	for i := 0; i < len(args); i++ {
		if args[i] == "-c" || args[i] == "--config" {
			i++
			continue
		}
		if strings.HasPrefix(args[i], "-") {
			continue
		}
		if args[i] == "exec" && i+1 < len(args) && args[i+1] == "resume" {
			start = i + 2
			break
		}
		switch args[i] {
		case "resume", "fork", "delete", "archive", "unarchive":
			start = i + 1
		}
		break
	}
	if start < 0 {
		return -1, args
	}
	for i := start; i < len(args); i++ {
		switch args[i] {
		case "--last":
			return -1, args
		case "-c", "--config", "-m", "--model", "-C", "--cd", "-i", "--image", "--enable", "--disable":
			i++
			continue
		}
		if strings.HasPrefix(args[i], "-") {
			continue
		}
		if len(args[i]) < 4 {
			return -1, args
		}
		for _, r := range strings.ToLower(args[i]) {
			if !strings.ContainsRune("0123456789abcdef-", r) {
				return -1, args
			}
		}
		return i, args
	}
	return -1, args
}
