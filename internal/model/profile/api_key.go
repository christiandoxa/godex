package profile

type APIKeyLoginInput struct {
	Name             string
	APIKey           string
	BaseURL          string
	BaseURLSpecified bool
}

type APIKeyLoginResult struct {
	Profile string
	Created bool
	Active  bool
	BaseURL string
}
