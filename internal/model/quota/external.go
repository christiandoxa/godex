package quota

type ExternalDetail struct {
	Label string
	Value string
}

type ExternalInfo struct {
	Provider         string
	Account          string
	Plan             string
	Status           string
	Main             string
	Reset            string
	ResetAt          *int64
	RemainingPercent *int64
	Available        *bool
	Details          []ExternalDetail
}

type VirtualResult struct {
	Name     string
	Provider string
	Auth     string
	External *ExternalInfo
	Err      error
}
