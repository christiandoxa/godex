package auth

import (
	"context"
	"strings"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
)

type fakeImporter struct{ name string }

func (fake *fakeImporter) Run(_ context.Context, name string) (accountentity.Account, error) {
	fake.name = name
	return accountentity.Account{ID: "synthetic-id", Name: "main", Email: "person@example.com"}, nil
}

func TestImportCurrentParsesOptionalName(t *testing.T) {
	importer := &fakeImporter{}
	var output strings.Builder
	if err := ImportCurrent(context.Background(), importer, &output, []string{"work"}); err != nil {
		t.Fatal(err)
	}
	if importer.name != "work" || !strings.Contains(output.String(), "Imported current Codex login as main (person@example.com).") {
		t.Fatalf("name/output = %q / %q", importer.name, output.String())
	}
	if err := ImportCurrent(context.Background(), importer, &output, []string{"one", "two"}); err == nil {
		t.Fatal("too many arguments unexpectedly accepted")
	}
}
