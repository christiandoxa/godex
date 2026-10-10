package cli

func shouldRunRuntimeHousekeeping(arguments []string) bool {
	if len(arguments) == 0 {
		return true
	}
	switch arguments[0] {
	case "info", "log", "ping", "update", "__runtime-broker":
		return false
	case "super", "s":
		for _, argument := range arguments[1:] {
			if argument == "--dry-run" {
				return false
			}
		}
	}
	return true
}
