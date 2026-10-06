package runtime

type Selection struct {
	Account               string
	Profile               string
	Provider              string
	CLI                   string
	DryRun                bool
	APIKey                string
	BaseURL               string
	URL                   string
	Model                 string
	ContextWindow         *uint64
	AutoCompactTokenLimit *uint64
	AutoRedeem            bool
	AutoRotate            bool
	NoAutoRotate          bool
	SkipQuotaCheck        bool
	NoProxy               bool
	FullAccess            bool
}

func (selection Selection) Empty() bool {
	return selection.Account == "" &&
		selection.Profile == "" &&
		selection.Provider == "" &&
		selection.CLI == "" &&
		!selection.DryRun &&
		selection.APIKey == "" &&
		selection.BaseURL == "" &&
		selection.URL == "" &&
		selection.Model == "" &&
		selection.ContextWindow == nil &&
		selection.AutoCompactTokenLimit == nil &&
		!selection.AutoRedeem &&
		!selection.AutoRotate &&
		!selection.NoAutoRotate &&
		!selection.SkipQuotaCheck &&
		!selection.NoProxy &&
		!selection.FullAccess
}
