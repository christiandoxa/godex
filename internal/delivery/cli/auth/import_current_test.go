package auth

import (
	"context"
	"strings"
	"testing"

	authmodel "github.com/christiandoxa/godex/internal/model/auth"
)

type fakeImporter struct {
	request authmodel.ImportCurrentRequest
}

func (fake *fakeImporter) Run(_ context.Context, request authmodel.ImportCurrentRequest) (authmodel.ImportCurrentResponse, error) {
	fake.request = request
	return authmodel.ImportCurrentResponse{ID: "synthetic-id", Name: "main", Email: "person@example.com"}, nil
}

func TestImportCurrentDefaultsToDefaultProfileName(t *testing.T) {
	importer := &fakeImporter{}
	var output strings.Builder
	if err := ImportCurrent(context.Background(), importer, &output, nil); err != nil {
		t.Fatal(err)
	}
	if importer.request.Name != "default" || importer.request.Insecure {
		t.Fatalf("default request = %+v", importer.request)
	}
}

func TestImportCurrentParsesOptionalName(t *testing.T) {
	importer := &fakeImporter{}
	var output strings.Builder
	if err := ImportCurrent(context.Background(), importer, &output, []string{"--insecure", "work"}); err != nil {
		t.Fatal(err)
	}
	if importer.request.Name != "work" || !importer.request.Insecure || !strings.Contains(output.String(), "Imported current Codex login as main (person@example.com).") {
		t.Fatalf("request/output = %+v / %q", importer.request, output.String())
	}
	if err := ImportCurrent(context.Background(), importer, &output, []string{"one", "two"}); err == nil {
		t.Fatal("too many arguments unexpectedly accepted")
	}
	if err := ImportCurrent(context.Background(), importer, &output, []string{"--unknown"}); err == nil {
		t.Fatal("unknown option unexpectedly accepted")
	}
}
