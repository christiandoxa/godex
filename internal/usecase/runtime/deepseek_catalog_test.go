package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	runtimerepo "github.com/christiandoxa/godex/internal/repository/runtime"
)

func TestDeepSeekProviderRuntimeArgumentsInjectDedicatedCatalog(t *testing.T) {
	home := t.TempDir()
	store := runtimerepo.NewProviderCatalogStore()
	provider := DeepSeekProvider("deepseek", "")
	arguments := []string{"--model=flash", "exec", "hello"}
	prepared, err := prepareProviderRuntimeArguments(store, home, provider, arguments)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(prepared, "\n")
	path := filepath.Join(home, proxymodel.DeepSeekModelCatalogFile)
	if !strings.Contains(joined, "model_catalog_json="+strconv.Quote(path)) {
		t.Fatalf("catalog argument missing: %#v", prepared)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Models) != 7 || envelope.Models[0]["slug"] != "flash" || envelope.Models[1]["slug"] != "auto" {
		t.Fatalf("DeepSeek catalog = %#v", envelope.Models)
	}
	first := envelope.Models[0]
	if first["context_window"] != float64(1_048_576) || first["auto_compact_token_limit"] != float64(900_000) || first["default_reasoning_level"] != "high" {
		t.Fatalf("DeepSeek launch catalog entry = %#v", first)
	}
}

func TestDeepSeekProviderRuntimeArgumentsRespectUserCatalogOverride(t *testing.T) {
	home := t.TempDir()
	custom := filepath.Join(t.TempDir(), "custom.json")
	prepared, err := prepareProviderRuntimeArguments(runtimerepo.NewProviderCatalogStore(), home, DeepSeekProvider("deepseek", ""), []string{
		"-c", "model_catalog_json=" + strconv.Quote(custom), "exec", "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, proxymodel.DeepSeekModelCatalogFile)); !os.IsNotExist(err) {
		t.Fatalf("managed DeepSeek catalog unexpectedly written: %v", err)
	}
	if strings.Count(strings.Join(prepared, "\n"), "model_catalog_json=") != 1 {
		t.Fatalf("catalog override duplicated: %#v", prepared)
	}
}
