package profile

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

type ProviderKind string

const (
	ProviderOpenAI    ProviderKind = "openai"
	ProviderGemini    ProviderKind = "gemini"
	ProviderAnthropic ProviderKind = "anthropic"
	ProviderCopilot   ProviderKind = "copilot"
	ProviderKiro      ProviderKind = "kiro"
	ProviderDeepSeek  ProviderKind = "deepseek"
	ProviderLocal     ProviderKind = "local"
	ProviderAgy       ProviderKind = "agy"
)

type Provider struct {
	Kind          ProviderKind `json:"provider_kind"`
	ProjectID     string       `json:"project_id,omitempty"`
	Account       string       `json:"account,omitempty"`
	AuthMethod    string       `json:"auth_method,omitempty"`
	Host          string       `json:"host,omitempty"`
	Login         string       `json:"login,omitempty"`
	APIURL        string       `json:"api_url,omitempty"`
	AccessTypeSKU string       `json:"access_type_sku,omitempty"`
	CopilotPlan   string       `json:"copilot_plan,omitempty"`
	AuthKey       string       `json:"auth_key,omitempty"`
	AuthKind      string       `json:"auth_kind,omitempty"`
	ProfileARN    string       `json:"profile_arn,omitempty"`
	ProfileName   string       `json:"profile_name,omitempty"`
	StartURL      string       `json:"start_url,omitempty"`
	Region        string       `json:"region,omitempty"`
}

type Profile struct {
	Name      string   `json:"name"`
	CodexHome string   `json:"codex_home"`
	Managed   bool     `json:"managed"`
	Email     string   `json:"email,omitempty"`
	Provider  Provider `json:"provider"`
}

type SourceKind int

const (
	SourceExternalHome SourceKind = iota
	SourceCopyFrom
	SourceCopyCurrent
	SourceEmptyManaged
)

func ValidateName(name string) error {
	if name == "" {
		return errors.New("profile name cannot be empty")
	}
	if name == "." || name == ".." {
		return errors.New("profile name cannot be '.' or '..'")
	}
	if strings.ContainsAny(name, `/\\`) {
		return errors.New("profile name cannot contain path separators")
	}
	for _, current := range name {
		if (current >= 'a' && current <= 'z') || (current >= 'A' && current <= 'Z') ||
			(current >= '0' && current <= '9') || current == '.' || current == '_' || current == '-' {
			continue
		}
		return errors.New("profile name may only contain letters, numbers, '.', '_' or '-'")
	}
	return nil
}

func Validate(value Profile) error {
	if err := ValidateName(value.Name); err != nil {
		return err
	}
	if strings.TrimSpace(value.CodexHome) == "" || !filepath.IsAbs(value.CodexHome) {
		return errors.New("profile CODEX_HOME must be an absolute path")
	}
	clean := filepath.Clean(value.CodexHome)
	if clean == filepath.Dir(clean) {
		return errors.New("profile CODEX_HOME must not be the filesystem root")
	}
	if !validProvider(value.Provider.Kind) {
		return fmt.Errorf("unsupported profile provider %q", value.Provider.Kind)
	}
	return nil
}

func ResolveSource(codexHome, copyFrom string, copyCurrent bool) (SourceKind, error) {
	hasHome := strings.TrimSpace(codexHome) != ""
	hasCopy := strings.TrimSpace(copyFrom) != ""
	if hasHome && (hasCopy || copyCurrent) {
		return 0, errors.New("--codex-home cannot be combined with --copy-from or --copy-current")
	}
	if hasCopy && copyCurrent {
		return 0, errors.New("use either --copy-from or --copy-current")
	}
	switch {
	case hasHome:
		return SourceExternalHome, nil
	case hasCopy:
		return SourceCopyFrom, nil
	case copyCurrent:
		return SourceCopyCurrent, nil
	default:
		return SourceEmptyManaged, nil
	}
}

func ShouldActivate(activeExists, requested bool) bool {
	return requested || !activeExists
}

func validProvider(kind ProviderKind) bool {
	switch kind {
	case ProviderOpenAI, ProviderGemini, ProviderAnthropic, ProviderCopilot, ProviderKiro, ProviderDeepSeek, ProviderLocal, ProviderAgy:
		return true
	default:
		return false
	}
}
