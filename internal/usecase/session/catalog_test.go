package session

import (
	"context"
	"errors"
	"reflect"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

type accountsFake struct{}

func (accountsFake) List(context.Context) ([]accountentity.Account, error) {
	return []accountentity.Account{{ID: "one", Name: "personal"}, {ID: "two", Name: "work"}}, nil
}
func (accountsFake) CodexHome(id string) string { return id }

type readerFake struct {
	reports map[string][]sessionentity.Session
}

func (f readerFake) List(_ context.Context, home string) ([]sessionentity.Session, error) {
	return f.reports[home], nil
}

type launcherFake struct {
	account string
	args    []string
}

func (f *launcherFake) Run(_ context.Context, account string, args []string) error {
	f.account = account
	f.args = args
	return nil
}
func testCatalog() (*Catalog, *launcherFake) {
	launcher := &launcherFake{}
	reader := readerFake{map[string][]sessionentity.Session{
		"one": {{ID: "a111", CWD: "/project", UpdatedUnix: 1, Path: "a"}},
		"two": {{ID: "b111", CWD: "/project", UpdatedUnix: 2, ThreadName: "Repair", Path: "b"}, {ID: "b222", CWD: "/other", UpdatedUnix: 3, ParentThreadID: "b111", Path: "c"}},
	}}
	return NewCatalog(accountsFake{}, reader, launcher), launcher
}
func TestListFiltersThenSortsAndLimits(t *testing.T) {
	catalog, _ := testCatalog()
	for _, test := range []struct {
		query sessionmodel.Query
		ids   []string
	}{
		{sessionmodel.Query{}, []string{"b222", "b111", "a111"}},
		{sessionmodel.Query{CurrentDir: "/project/.", Limit: 1}, []string{"b111"}},
		{sessionmodel.Query{Profile: "WORK", Text: "repair"}, []string{"b111"}},
		{sessionmodel.Query{ParentOnly: true}, []string{"b111", "a111"}},
		{sessionmodel.Query{LimitSet: true}, []string{}},
	} {
		reports, err := catalog.List(context.Background(), test.query)
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]string, 0, len(reports))
		for _, r := range reports {
			ids = append(ids, r.ID)
		}
		if !reflect.DeepEqual(ids, test.ids) {
			t.Fatalf("query %#v = %v, want %v", test.query, ids, test.ids)
		}
	}
	if _, err := catalog.List(context.Background(), sessionmodel.Query{Profile: "missing"}); err == nil {
		t.Fatal("missing profile accepted")
	}
}
func TestResumeResolvesPrefixAndPreservesOwner(t *testing.T) {
	catalog, launcher := testCatalog()
	if err := catalog.Resume(context.Background(), "b1"); err != nil {
		t.Fatal(err)
	}
	if launcher.account != "two" || !reflect.DeepEqual(launcher.args, []string{"resume", "b111"}) {
		t.Fatalf("launch = %#v", launcher)
	}
	for _, selector := range []string{"", "missing", "b"} {
		if _, err := catalog.Resolve(context.Background(), selector); err == nil {
			t.Fatalf("accepted %q", selector)
		}
	}
	catalog.reader = readerFake{map[string][]sessionentity.Session{"one": {{ID: "duplicate"}}, "two": {{ID: "duplicate"}}}}
	if _, err := catalog.Resolve(context.Background(), "duplicate"); err == nil {
		t.Fatal("ambiguous ownership accepted")
	}
	catalog.launcher = nil
	if err := catalog.Resume(context.Background(), "duplicate"); err == nil || errors.Is(err, context.Canceled) {
		t.Fatalf("missing launcher = %v", err)
	}
}

func (f *launcherFake) RunLocal(ctx context.Context, account string, args []string) error {
	return f.Run(ctx, account, args)
}

func TestNativeSessionPreservesArgumentsAndRejectsSelectorConflict(t *testing.T) {
	catalog, launcher := testCatalog()
	args := []string{"exec", "resume", "b1", "continue", "--json"}
	if err := catalog.ResumeArguments(context.Background(), "work", "b1", 2, args); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(launcher.args, []string{"exec", "resume", "b111", "continue", "--json"}) {
		t.Fatalf("arguments = %v", launcher.args)
	}
	if err := catalog.ResumeArguments(context.Background(), "personal", "b1", 2, args); err == nil {
		t.Fatal("conflicting account accepted")
	}
}
