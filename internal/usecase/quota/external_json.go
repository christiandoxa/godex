package quota

import (
	"bytes"
	"encoding/json"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type externalQuotaJSON struct {
	Provider  string                    `json:"provider"`
	Account   *string                   `json:"account"`
	Plan      *string                   `json:"plan"`
	Status    string                    `json:"status"`
	Main      string                    `json:"main"`
	Reset     *string                   `json:"reset"`
	Available *bool                     `json:"available"`
	Details   []externalQuotaDetailJSON `json:"details"`
}

type externalQuotaDetailJSON struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

func marshalExternalQuotaJSON(info quotamodel.ExternalInfo) ([]byte, error) {
	details := make([]externalQuotaDetailJSON, 0, len(info.Details))
	for _, detail := range info.Details {
		details = append(details, externalQuotaDetailJSON{Label: detail.Label, Value: detail.Value})
	}
	return marshalQuotaJSON(externalQuotaJSON{
		Provider: info.Provider, Account: optionalExternalString(info.Account),
		Plan: optionalExternalString(info.Plan), Status: info.Status, Main: info.Main,
		Reset: optionalExternalString(info.Reset), Available: info.Available, Details: details,
	})
}

func optionalExternalString(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}

func marshalQuotaJSON(value any) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(output.Bytes(), []byte{'\n'}), nil
}
