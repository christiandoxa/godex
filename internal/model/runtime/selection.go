package runtime

type Selection struct {
	Account string
	Profile string
}

func (selection Selection) Empty() bool {
	return selection.Account == "" && selection.Profile == ""
}
