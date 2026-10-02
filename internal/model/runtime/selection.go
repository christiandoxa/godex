package runtime

type Selection struct {
	Account               string
	Profile               string
	Provider              string
	APIKey                string
	BaseURL               string
	URL                   string
	Model                 string
	ContextWindow         *uint64
	AutoCompactTokenLimit *uint64
}

func (selection Selection) Empty() bool {
	return selection.Account == "" &&
		selection.Profile == "" &&
		selection.Provider == "" &&
		selection.APIKey == "" &&
		selection.BaseURL == "" &&
		selection.URL == "" &&
		selection.Model == "" &&
		selection.ContextWindow == nil &&
		selection.AutoCompactTokenLimit == nil
}
