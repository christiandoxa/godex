package profile

type ExportRequest struct {
	Profiles   []string
	OutputPath string
	Password   string
}

type ExportResult struct {
	ProfileCount  int
	Path          string
	Encrypted     bool
	ActiveProfile string
}

type ImportRequest struct {
	Path     string
	Password string
}

type ImportResult struct {
	ImportedCount int
	UpdatedCount  int
	Path          string
	Encrypted     bool
	ActiveProfile string
}

type BundlePayload struct {
	ExportedAt          string            `json:"exported_at"`
	SourceProdexVersion string            `json:"source_prodex_version"`
	ActiveProfile       *string           `json:"active_profile"`
	Profiles            []ExportedProfile `json:"profiles"`
}

type ExportedProfile struct {
	Name          string               `json:"name"`
	Email         *string              `json:"email"`
	SourceManaged bool                 `json:"source_managed"`
	Provider      ProviderSnapshot     `json:"provider"`
	AuthJSON      string               `json:"auth_json"`
	SecretFiles   []ExportedSecretFile `json:"secret_files"`
}

type ProviderSnapshot struct {
	Kind          string  `json:"provider_kind"`
	Email         *string `json:"email,omitempty"`
	ProjectID     *string `json:"project_id,omitempty"`
	Account       *string `json:"account,omitempty"`
	AuthMethod    *string `json:"auth_method,omitempty"`
	Host          *string `json:"host,omitempty"`
	Login         *string `json:"login,omitempty"`
	APIURL        *string `json:"api_url,omitempty"`
	AccessTypeSKU *string `json:"access_type_sku,omitempty"`
	CopilotPlan   *string `json:"copilot_plan,omitempty"`
	AuthKey       *string `json:"auth_key,omitempty"`
	AuthKind      *string `json:"auth_kind,omitempty"`
	ProfileARN    *string `json:"profile_arn,omitempty"`
	ProfileName   *string `json:"profile_name,omitempty"`
	StartURL      *string `json:"start_url,omitempty"`
	Region        *string `json:"region,omitempty"`
}

type ExportedSecretFile struct {
	Path string `json:"path"`
	Text string `json:"text"`
}
