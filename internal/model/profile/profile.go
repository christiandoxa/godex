package profile

type AddRequest struct {
	Name        string
	CodexHome   string
	CopyFrom    string
	CopyCurrent bool
	Activate    bool
	Insecure    bool
}

type RemoveRequest struct {
	Name       string
	All        bool
	DeleteHome bool
}

type LaunchTarget struct {
	Name           string
	CodexHome      string
	AccountID      string
	Provider       string
	Auth           string
	ProviderConfig ProviderSnapshot
}

type Summary struct {
	Count  int
	Active string
}
