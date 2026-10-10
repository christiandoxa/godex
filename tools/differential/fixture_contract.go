package main

import "encoding/json"

// Verify the stable mock fixture independently. Equality between Prodex and
// Godex alone would miss identical corruption in both products or in the shim.
func validFixtureRequest(body string) bool {
	return validFixtureRequestMode(body, false)
}

func validFixtureStreamingRequest(body string) bool {
	return validFixtureRequestMode(body, true)
}

func validFixtureRequestMode(body string, wantStream bool) bool {
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
		request.Stream == nil || *request.Stream != wantStream ||
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

// A successful tool-call response has a specific stable call identity and
// structured arguments. Do not count an ordinary text answer as equivalent.
func validFixtureToolCallResponse(body string) bool {
	var decoded struct {
		Object string `json:"object"`
		Model  string `json:"model"`
		Output []struct {
			Type      string          `json:"type"`
			CallID    string          `json:"call_id"`
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"output"`
		Usage struct {
			Input  int `json:"input_tokens"`
			Output int `json:"output_tokens"`
			Total  int `json:"total_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal([]byte(body), &decoded) != nil ||
		decoded.Object != "response" || decoded.Model != "deepseek-v4-pro" ||
		decoded.Usage.Input != 2 || decoded.Usage.Output != 1 || decoded.Usage.Total != 3 ||
		len(decoded.Output) != 1 {
		return false
	}
	call := decoded.Output[0]
	if call.Type != "function_call" || call.CallID != "call-differential" || call.Name != "lookup" {
		return false
	}
	var serialized string
	if json.Unmarshal(call.Arguments, &serialized) != nil {
		return false
	}
	var args map[string]any
	if json.Unmarshal([]byte(serialized), &args) != nil || len(args) != 1 {
		return false
	}
	return args["key"] == "alpha"
}

// Provider authentication failures must reach the client with the authentic
// error code. Matching HTTP status alone does not prove the error was preserved.
func validFixtureError(body, expectedCode string) bool {
	var root struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(body), &root) != nil || root.Error.Code != expectedCode {
		return false
	}
	switch expectedCode {
	case "invalid_api_key":
		return root.Error.Message == "synthetic credential rejected"
	case "access_denied":
		return root.Error.Message == "synthetic provider forbidden"
	case "internal_server_error":
		return root.Error.Message == "synthetic provider failure"
	}
	return false
}
