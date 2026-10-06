package runtime

import (
	"fmt"
	"strings"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type kiroCatalogBuilder struct {
	entries       []proxymodel.ProviderCatalogEntry
	contextWindow uint64
	autoCompact   uint64
	models        []map[string]any
	seen          map[string]bool
}

func buildKiroExternalCatalog(
	store providerCatalogStore,
	home string,
	provider proxymodel.Provider,
	arguments []string,
) ([]map[string]any, error) {
	entries, err := proxymodel.KiroProviderCatalog()
	if err != nil {
		return nil, err
	}
	dynamic, err := store.ReadKiroProfile(home)
	if err != nil {
		return nil, err
	}
	builder, err := newKiroCatalogBuilder(entries, dynamic, provider, arguments)
	if err != nil {
		return nil, err
	}
	if err := builder.appendLaunch(effectiveProviderModel(provider, arguments)); err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if err := builder.appendEntry(entry); err != nil {
			return nil, err
		}
	}
	for _, item := range dynamic {
		if err := builder.appendDynamic(item); err != nil {
			return nil, err
		}
	}
	return builder.models, nil
}

func newKiroCatalogBuilder(
	entries []proxymodel.ProviderCatalogEntry,
	dynamic []map[string]any,
	provider proxymodel.Provider,
	arguments []string,
) (*kiroCatalogBuilder, error) {
	contextWindow, err := effectiveProviderUint(arguments, "model_context_window", uint64(provider.ContextWindow))
	if err != nil {
		return nil, err
	}
	autoCompact, err := effectiveProviderUint(arguments, "model_auto_compact_token_limit", uint64(provider.AutoCompactLimit))
	if err != nil {
		return nil, err
	}
	capacity := len(entries) + len(dynamic) + 1
	return &kiroCatalogBuilder{
		entries: entries, contextWindow: contextWindow, autoCompact: autoCompact,
		models: make([]map[string]any, 0, capacity), seen: make(map[string]bool, capacity),
	}, nil
}

func (builder *kiroCatalogBuilder) appendLaunch(model string) error {
	if entry := proxymodel.ResolveProviderCatalogEntry(builder.entries, model); entry != nil {
		return builder.appendEntry(*entry)
	}
	return builder.appendModel(model, model, "", 0)
}

func (builder *kiroCatalogBuilder) appendEntry(entry proxymodel.ProviderCatalogEntry) error {
	return builder.appendModel(entry.ID, entry.DisplayName, entry.Description, catalogEntryContext(entry))
}

func (builder *kiroCatalogBuilder) appendDynamic(item map[string]any) error {
	return builder.appendModel(
		catalogMapString(item, "id", "model_id", "modelId", "slug", "model"),
		catalogMapString(item, "display_name", "name", "model_name", "modelName"),
		catalogMapString(item, "description"),
		catalogMapUint(item, "context_window_tokens", "contextWindowTokens", "context_window"),
	)
}

func (builder *kiroCatalogBuilder) appendModel(slug, display, description string, modelContext uint64) error {
	slug = strings.TrimSpace(slug)
	key := strings.ToLower(slug)
	if slug == "" || builder.seen[key] {
		return nil
	}
	if len(builder.models) >= proxymodel.ProviderCatalogMaxItems {
		return fmt.Errorf("provider model catalog exceeds the hard limit of %d entries", proxymodel.ProviderCatalogMaxItems)
	}
	builder.seen[key] = true
	if modelContext == 0 {
		modelContext = builder.contextWindow
	}
	compact := builder.autoCompact
	if modelContext > 0 {
		compact = modelContext * 95 / 100
	}
	builder.models = append(builder.models, externalCodexCatalogModel(
		slug, valueOr(display, slug), valueOr(description, "Kiro model exposed through the Godex Responses adapter."),
		len(builder.models)+1, modelContext, compact, builder.entries,
	))
	return nil
}

func catalogEntryContext(entry proxymodel.ProviderCatalogEntry) uint64 {
	if entry.ContextWindowTokens == nil {
		return 0
	}
	return *entry.ContextWindowTokens
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
