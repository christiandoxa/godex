package gemini

import "github.com/google/uuid"

func geminiResponseIdentity04384(root map[string]any, chatCompatible bool) (responseID, model string) {
	// Source 0.438.4 has distinct identity contracts here. The API-key
	// provider translator accepts only responseId/modelVersion, defaulting
	// to gemini_resp_prodex/gemini-2.5-pro. Native runtime accepts the id
	// and model aliases only when their primary members are ABSENT. Empty
	// strings are valid values; present null/wrong-type fields suppress
	// aliases rather than borrowing another upstream identity.
	rawID, idPresent := root["responseId"]
	if !idPresent && !chatCompatible {
		rawID = root["id"]
	}
	responseID, idValid := rawID.(string)
	if !idValid {
		if chatCompatible {
			responseID = "gemini_resp_prodex"
		} else {
			responseID = "resp_gemini_" + uuid.NewString()
		}
	}
	rawModel, modelPresent := root["modelVersion"]
	if !modelPresent && !chatCompatible {
		rawModel = root["model"]
	}
	model, modelValid := rawModel.(string)
	if !modelValid {
		if chatCompatible {
			model = "gemini-2.5-pro"
		} else {
			model = "auto"
		}
	}

	return responseID, model
}
