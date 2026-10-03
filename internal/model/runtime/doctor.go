package runtime

import "encoding/json"

type DoctorOptions struct {
	Runtime                  bool
	Quota                    bool
	Install                  bool
	RepairImportAuthJournals bool
	TailBytes                int
}

type DoctorDiagnostics struct {
	GeneratedAt        string                    `json:"generated_at"`
	GodexHome          string                    `json:"godex_home"`
	CodexVersion       string                    `json:"codex_version"`
	AccountCount       int                       `json:"account_count"`
	EnabledCount       int                       `json:"enabled_count"`
	ImportAuthJournals *DoctorImportAuthJournals `json:"import_auth_journals,omitempty"`
	Install            []DoctorCheck             `json:"install_checks,omitempty"`
	Runtime            *DoctorRuntime            `json:"runtime,omitempty"`
	Quota              []DoctorQuota             `json:"quota_probes,omitempty"`
}

type DoctorImportAuthJournals struct {
	OrphanCount     int    `json:"orphan_count"`
	RepairPerformed bool   `json:"repair_performed"`
	Repaired        int    `json:"repaired"`
	Status          string `json:"status"`
}

type DoctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type DoctorRuntime struct {
	Overview  Overview `json:"overview"`
	Events    []Event  `json:"events"`
	TailBytes int      `json:"tail_bytes"`
}

type DoctorQuota struct {
	Profile  string               `json:"profile"`
	Provider string               `json:"provider"`
	Auth     string               `json:"auth"`
	State    string               `json:"state"`
	External *DoctorExternalQuota `json:"-"`
	OpenAI   *DoctorOpenAIQuota   `json:"-"`
	Error    string               `json:"-"`
	Plan     string               `json:"plan,omitempty"`
	FiveHour string               `json:"five_hour,omitempty"`
	Weekly   string               `json:"weekly,omitempty"`
	Active   bool                 `json:"active"`
	Enabled  bool                 `json:"enabled"`
}

type DoctorExternalQuota struct {
	Status string
	Main   string
	Reset  string
}

type DoctorOpenAIQuota struct {
	Status      string
	HumanStatus string
	Main        string
}

func (quota DoctorQuota) MarshalJSON() ([]byte, error) {
	if quota.Error != "" {
		return json.Marshal(struct {
			Profile  string         `json:"profile"`
			Provider string         `json:"provider"`
			Quota    map[string]any `json:"quota"`
		}{
			Profile: quota.Profile, Provider: quota.Provider,
			Quota: map[string]any{"error": quota.Error},
		})
	}
	if quota.OpenAI != nil {
		return json.Marshal(struct {
			Profile  string         `json:"profile"`
			Provider string         `json:"provider"`
			Quota    map[string]any `json:"quota"`
		}{
			Profile: quota.Profile, Provider: quota.Provider,
			Quota: map[string]any{"status": quota.OpenAI.Status, "main": quota.OpenAI.Main},
		})
	}
	if quota.External == nil {
		type doctorQuotaAlias DoctorQuota
		return json.Marshal(doctorQuotaAlias(quota))
	}
	var reset any
	if quota.External.Reset != "" {
		reset = quota.External.Reset
	}
	return json.Marshal(struct {
		Profile  string         `json:"profile"`
		Provider string         `json:"provider"`
		Quota    map[string]any `json:"quota"`
	}{
		Profile: quota.Profile, Provider: quota.Provider,
		Quota: map[string]any{
			"status": quota.External.Status, "main": quota.External.Main, "reset": reset,
		},
	})
}
