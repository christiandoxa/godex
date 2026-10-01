package kiro

import (
	"net/url"
	"testing"
)

func TestReadOnlySQLiteDSNForWindowsDrive(t *testing.T) {
	dsn := readOnlySQLiteDSNForOS(`C:\Users\Test User\data.sqlite3`, "windows")
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "file" || parsed.Host != "" || parsed.Path != "/C:/Users/Test User/data.sqlite3" {
		t.Fatalf("parsed Windows DSN = %#v (%s)", parsed, dsn)
	}
	assertReadOnlySQLiteQuery(t, parsed)
}

func TestReadOnlySQLiteDSNForUnixPath(t *testing.T) {
	dsn := readOnlySQLiteDSNForOS("/tmp/Test User/data.sqlite3", "linux")
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "file" || parsed.Host != "" || parsed.Path != "/tmp/Test User/data.sqlite3" {
		t.Fatalf("parsed Unix DSN = %#v (%s)", parsed, dsn)
	}
	assertReadOnlySQLiteQuery(t, parsed)
}

func assertReadOnlySQLiteQuery(t *testing.T, parsed *url.URL) {
	t.Helper()
	query := parsed.Query()
	if query.Get("mode") != "ro" {
		t.Fatalf("mode = %q", query.Get("mode"))
	}
	pragmas := query["_pragma"]
	if len(pragmas) != 2 {
		t.Fatalf("pragmas = %#v", pragmas)
	}
	found := map[string]bool{}
	for _, pragma := range pragmas {
		found[pragma] = true
	}
	if !found["query_only(1)"] || !found["busy_timeout(2000)"] {
		t.Fatalf("pragmas = %#v", pragmas)
	}
}
