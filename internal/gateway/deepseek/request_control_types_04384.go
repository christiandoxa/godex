package deepseek

import "fmt"

func validateDeepSeekControlTypes04384(request map[string]any) error {
	if value, present := request["stream"]; present {
		if _, valid := value.(bool); !valid {
			return &invalidResponsesInputError{fmt.Errorf("DeepSeek stream must be a boolean")}
		}
	}
	for _, field := range []string{"instructions", "previous_response_id"} {
		if value, present := request[field]; present {
			if _, valid := value.(string); !valid {
				return &invalidResponsesInputError{fmt.Errorf("DeepSeek %s must be a string", field)}
			}
		}
	}
	return nil
}
