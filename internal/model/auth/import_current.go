package auth

type ImportCurrentRequest struct {
	Name     string
	Insecure bool
}

type ImportCurrentIdentity struct {
	Email            string
	ChatGPTAccountID string
}

type ImportCurrentResponse struct {
	ID    string
	Name  string
	Email string
}
