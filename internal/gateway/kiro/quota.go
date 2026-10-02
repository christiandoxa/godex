package kiro

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func (source *Source) FetchQuota(ctx context.Context, target profilemodel.QuotaTarget) (quotamodel.ExternalInfo, error) {
	secretText, found, err := readManagedQuotaFile(target.CodexHome, CredentialsFile)
	if err != nil {
		return quotamodel.ExternalInfo{}, err
	}
	if !found {
		return quotamodel.ExternalInfo{}, errors.New("Kiro quota requires an imported auth snapshot")
	}
	credential, err := source.InspectAuthSecret(ctx, secretText)
	if err != nil {
		return quotamodel.ExternalInfo{}, err
	}
	modelCount, err := source.quotaModelCount(ctx, target.CodexHome)
	if err != nil {
		return quotamodel.ExternalInfo{}, err
	}
	provider := credential.Provider
	account := strings.TrimSpace(credential.Email)
	if account == "" {
		account = firstQuotaValue(provider.ProfileName, provider.ProfileARN)
	}
	authKind := quotaPointerValue(provider.AuthKind)
	profileName := quotaPointerValue(provider.ProfileName)
	region := quotaPointerValue(provider.Region)
	details := []quotamodel.ExternalDetail{{Label: "Auth method", Value: authKind}}
	if profileName != "" {
		details = append(details, quotamodel.ExternalDetail{Label: "Profile", Value: profileName})
	}
	if region != "" {
		details = append(details, quotamodel.ExternalDetail{Label: "Region", Value: region})
	}
	modelsValue := "catalog unavailable"
	main := "credential snapshot available"
	if modelCount != nil {
		modelsValue = fmt.Sprintf("%d", *modelCount)
		main = fmt.Sprintf("%d imported models", *modelCount)
	}
	details = append(details, quotamodel.ExternalDetail{Label: "Models", Value: modelsValue})
	available := true
	return quotamodel.ExternalInfo{
		Provider: "Kiro CLI", Account: account, Plan: authKind,
		Status: "Ready (imported)", Main: main, Available: &available, Details: details,
	}, nil
}

func (source *Source) quotaModelCount(ctx context.Context, home string) (*int, error) {
	text, found, err := readManagedQuotaFile(home, ModelCatalogFile)
	if err != nil || !found {
		return nil, err
	}
	if err := source.ValidateModelCatalog(ctx, text); err != nil {
		return nil, fmt.Errorf("failed to parse Kiro model catalog: %w", err)
	}
	var value any
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		return nil, errors.New("failed to parse Kiro model catalog")
	}
	models, ok := findModels(value)
	if !ok {
		return nil, errors.New("failed to parse Kiro model catalog")
	}
	count := len(normalizeModels(models))
	return &count, nil
}

func readManagedQuotaFile(home, name string) (string, bool, error) {
	if strings.TrimSpace(home) == "" || !filepath.IsAbs(home) {
		return "", false, errors.New("Kiro quota profile home must be absolute")
	}
	path := filepath.Join(filepath.Clean(home), name)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, errors.New("failed to inspect Kiro quota snapshot")
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > secretMaxBytes {
		return "", false, errors.New("Kiro quota snapshot must be a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", false, errors.New("failed to open Kiro quota snapshot")
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, secretMaxBytes+1))
	if err != nil || len(content) > secretMaxBytes {
		return "", false, errors.New("failed to read Kiro quota snapshot")
	}
	return string(content), true, nil
}

func firstQuotaValue(values ...*string) string {
	for _, value := range values {
		if current := quotaPointerValue(value); current != "" {
			return current
		}
	}
	return ""
}

func quotaPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
