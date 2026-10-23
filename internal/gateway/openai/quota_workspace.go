package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/jsonutil"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func (client *QuotaClient) FetchWorkspaceAt(ctx context.Context, home, upstream string) (*quotamodel.Workspace, error) {
	auth, err := client.auth.ReadAuth(ctx, home)
	if err != nil || auth.AccountID == "" {
		return nil, err
	}
	workspace := &quotamodel.Workspace{ID: auth.AccountID}
	endpoint, err := client.usageURL(upstream)
	if err != nil {
		return workspace, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(endpoint, "/usage")+"/accounts/check", nil)
	if err != nil {
		return workspace, err
	}
	headers, err := client.auth.ReadQuotaHeaders(ctx, home)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	request.Header.Set("ChatGPT-Account-Id", auth.AccountID)
	request.Header.Set("originator", headers.Originator)
	if headers.Residency != "" {
		request.Header.Set("x-openai-internal-codex-residency", headers.Residency)
	}
	response, err := client.client.Do(request)
	if err != nil {
		return workspace, nil
	}
	defer response.Body.Close()
	const maxWorkspaceBytes = 8 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, maxWorkspaceBytes+1))
	if err == nil && len(body) <= maxWorkspaceBytes && response.StatusCode >= 200 && response.StatusCode < 300 {
		workspace.Name = quotaWorkspaceName(body, auth.AccountID)
	}
	return workspace, nil
}

func quotaWorkspaceName(body []byte, accountID string) string {
	if !json.Valid(body) || jsonutil.CheckSerdeBoundary(string(body)) != nil || !jsonutil.CheckSerdeNumberTokens(body) {
		return ""
	}
	fields, ok := knownJSONFields(body, "accounts", "account_ordering")
	if !ok {
		return ""
	}
	var rawOrdering []*string
	if raw, exists := fields["account_ordering"]; exists && (string(raw) == "null" || json.Unmarshal(raw, &rawOrdering) != nil) {
		return ""
	}
	ordering := make([]string, 0, len(rawOrdering))
	for _, key := range rawOrdering {
		if key == nil {
			return ""
		}
		ordering = append(ordering, *key)
	}
	entries, ok := quotaWorkspaceEntries(fields["accounts"], ordering)
	if !ok {
		return ""
	}
	name, found := "", false
	for _, entry := range entries {
		if entry.id == nil || *entry.id != accountID || found {
			continue
		}
		found = true
		if entry.name != nil {
			name = strings.TrimSpace(*entry.name)
		}
		if name == "" && entry.structure != nil && strings.ToLower(strings.TrimSpace(*entry.structure)) == "personal" {
			name = "Personal"
		}
	}
	return name
}

type quotaWorkspaceEntry struct{ id, name, structure *string }

func parseWorkspaceEntry(raw json.RawMessage, idKey string) (quotaWorkspaceEntry, bool) {
	fields, ok := knownJSONFields(raw, idKey, "name", "structure")
	if !ok {
		return quotaWorkspaceEntry{}, false
	}
	entry := quotaWorkspaceEntry{}
	for _, field := range []struct {
		key string
		out **string
	}{{idKey, &entry.id}, {"name", &entry.name}, {"structure", &entry.structure}} {
		if value, exists := fields[field.key]; exists && json.Unmarshal(value, field.out) != nil {
			return quotaWorkspaceEntry{}, false
		}
	}
	return entry, idKey != "id" || entry.id != nil
}

func quotaWorkspaceEntries(raw json.RawMessage, ordering []string) ([]quotaWorkspaceEntry, bool) {
	var result []quotaWorkspaceEntry
	if len(raw) == 0 {
		return result, true
	}
	if raw[0] == '[' {
		var list []json.RawMessage
		if json.Unmarshal(raw, &list) != nil {
			return nil, false
		}
		for _, raw := range list {
			entry, ok := parseWorkspaceEntry(raw, "id")
			if !ok {
				return nil, false
			}
			result = append(result, entry)
		}
		return result, true
	}
	var wrappers map[string]json.RawMessage
	if string(raw) == "null" || json.Unmarshal(raw, &wrappers) != nil {
		return nil, false
	}
	accounts := make(map[string]quotaWorkspaceEntry, len(wrappers))
	for key, wrapper := range wrappers {
		fields, ok := knownJSONFields(wrapper, "account")
		if !ok {
			return nil, false
		}
		entry, ok := parseWorkspaceEntry(fields["account"], "account_id")
		if !ok {
			return nil, false
		}
		accounts[key] = entry
	}
	for _, key := range ordering {
		if entry, exists := accounts[key]; exists {
			result = append(result, entry)
			delete(accounts, key)
		}
	}
	if len(result) == 0 {
		for _, entry := range accounts {
			result = append(result, entry)
		}
	}
	return result, true
}
