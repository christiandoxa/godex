package ping

import (
	"encoding/json"
	"io"

	pingmodel "github.com/christiandoxa/godex/internal/model/ping"
)

type jsonResult struct {
	Profile                string           `json:"profile"`
	Status                 pingmodel.Status `json:"status"`
	Model                  *string          `json:"model"`
	RequestedModel         *string          `json:"requested_model"`
	EffectiveModel         *string          `json:"effective_model"`
	CredentialValidation   string           `json:"credential_validation"`
	FirstResponseLatencyMS *int64           `json:"first_response_latency_ms"`
	CompletionLatencyMS    *int64           `json:"completion_latency_ms"`
	LatencyMS              *int64           `json:"latency_ms"`
	Detail                 string           `json:"detail"`
}

type jsonReport struct {
	Provider       string            `json:"provider"`
	Status         string            `json:"status"`
	Model          *string           `json:"model"`
	RequestedModel *string           `json:"requested_model"`
	EffectiveModel *string           `json:"effective_model"`
	LatencyMS      int64             `json:"latency_ms"`
	Detail         string            `json:"detail"`
	Profiles       []jsonResult      `json:"profiles"`
	Summary        pingmodel.Summary `json:"summary"`
}

func writeJSON(out io.Writer, report pingmodel.Report) error {
	value := jsonReport{
		Provider: report.Provider, Status: report.Status,
		Model: optionalString(report.Model), RequestedModel: optionalString(report.RequestedModel),
		EffectiveModel: optionalString(report.EffectiveModel), LatencyMS: report.LatencyMS,
		Detail: report.Detail, Summary: report.Summary,
		Profiles: make([]jsonResult, 0, len(report.Profiles)),
	}
	for _, result := range report.Profiles {
		value.Profiles = append(value.Profiles, jsonResult{
			Profile: result.Profile, Status: result.Status,
			Model: optionalString(result.Model), RequestedModel: optionalString(result.RequestedModel),
			EffectiveModel: optionalString(result.EffectiveModel), CredentialValidation: result.CredentialValidation,
			FirstResponseLatencyMS: result.FirstResponseLatencyMS, CompletionLatencyMS: result.CompletionLatencyMS,
			LatencyMS: result.LatencyMS, Detail: result.Detail,
		})
	}
	return json.NewEncoder(out).Encode(value)
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}
