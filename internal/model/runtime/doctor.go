package runtime

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
	Profile  string `json:"profile"`
	Provider string `json:"provider"`
	Auth     string `json:"auth"`
	State    string `json:"state"`
	Plan     string `json:"plan,omitempty"`
	FiveHour string `json:"five_hour,omitempty"`
	Weekly   string `json:"weekly,omitempty"`
	Active   bool   `json:"active"`
	Enabled  bool   `json:"enabled"`
}
