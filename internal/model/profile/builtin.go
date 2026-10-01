package profile

type BuiltinImportRequest struct {
	Source   string
	Name     string
	Activate bool
	Insecure bool
}

type BuiltinCredential struct {
	Provider    ProviderSnapshot
	Email       string
	SecretFiles []ExportedSecretFile
}

type BuiltinImportResult struct {
	Profile  string
	Provider string
	Updated  bool
	Active   bool
}
