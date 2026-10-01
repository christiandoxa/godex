package runtime

type Selection struct {
	Account  string
	Profile  string
	Provider string
	APIKey   string
	BaseURL  string
}

func (selection Selection) Empty() bool {
	return selection.Account == "" &&
		selection.Profile == "" &&
		selection.Provider == "" &&
		selection.APIKey == "" &&
		selection.BaseURL == ""
}
