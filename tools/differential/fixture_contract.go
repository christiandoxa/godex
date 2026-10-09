package main

import "encoding/json"

// Verify the stable mock fixture independently. Equality between Prodex and
// Godex alone would miss identical corruption in both products or in the shim.
func validFixtureRequest(body string) bool {
	var request struct {
		Model    string `json:"model"`
		Stream   *bool  `json:"stream"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if json.Unmarshal([]byte(body), &request) != nil ||
		request.Model != "deepseek-v4-pro" ||
		request.Stream == nil || *request.Stream ||
		len(request.Messages) != 1 {
		return false
	}
	return request.Messages[0].Role == "user" &&
		request.Messages[0].Content == "same request"
}

func validFixtureResponse(body string) bool {
	var response struct {
		Object string `json:"object"`
		Model  string `json:"model"`
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Usage struct {
			Input  int `json:"input_tokens"`
			Output int `json:"output_tokens"`
			Total  int `json:"total_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal([]byte(body), &response) != nil ||
		response.Object != "response" || response.Model != "deepseek-v4-pro" ||
		response.Usage.Input != 2 || response.Usage.Output != 1 ||
		response.Usage.Total != 3 || len(response.Output) != 1 ||
		response.Output[0].Type != "message" ||
		len(response.Output[0].Content) != 1 {
		return false
	}
	content := response.Output[0].Content[0]
	return content.Type == "output_text" && content.Text == "synthetic-ok"
}
