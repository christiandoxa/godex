package superexpose

import (
	"errors"
	"os"
	"unicode/utf8"
)

func openAITunnelAPIKeyFromEnv() (string, error) {
	apiKey, present := os.LookupEnv("CONTROL_PLANE_API_KEY")
	if !present || !utf8.ValidString(apiKey) {
		return "", errors.New("OpenAI Secure MCP Tunnel requires CONTROL_PLANE_API_KEY in noninteractive mode")
	}
	if apiKey == "" || len(apiKey) > openAITunnelAPIKeyMax || tunnelHasControl(apiKey) {
		return "", errors.New("OpenAI Secure MCP Tunnel API key is invalid")
	}
	return apiKey, nil
}
