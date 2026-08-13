package runtime

import "errors"

func parseRunArguments(arguments []string) (string, []string, error) {
	selector := ""
	for index := 0; index < len(arguments); index++ {
		switch arguments[index] {
		case "--":
			return selector, arguments[index+1:], nil
		case "--account":
			if index+1 >= len(arguments) {
				return "", nil, errors.New("--account requires a selector")
			}
			selector = arguments[index+1]
			index++
		default:
			return selector, arguments[index:], nil
		}
	}
	return selector, nil, nil
}
