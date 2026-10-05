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
	local   bool
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
	f.local = true
	return f.Run(ctx, account, args)
}

func TestNativeSessionPreservesArgumentsAndRejectsSelectorConflict(t *testing.T) {
	catalog, launcher := testCatalog()
	args := []string{"exec", "resume", "b1", "continue", "--json"}
	if err := catalog.ResumeArguments(context.Background(), sessionmodel.Launch{AccountSelector: "work", SessionSelector: "b1", IDIndex: 2, Arguments: args}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(launcher.args, []string{"exec", "resume", "b111", "continue", "--json"}) {
		t.Fatalf("arguments = %v", launcher.args)
	}
	if err := catalog.ResumeArguments(context.Background(), sessionmodel.Launch{AccountSelector: "personal", SessionSelector: "b1", IDIndex: 2, Arguments: args}); err == nil {
		t.Fatal("conflicting account accepted")
	}
}

func (f *launcherFake) RunSession(ctx context.Context, home, owner string, args []string) error {
	return f.Run(ctx, home, args)
}

func TestSessionLocalIntentSurvivesRootOptions(t *testing.T) {
	catalog, launcher := testCatalog()
	input := sessionmodel.Launch{SessionSelector: "b1", IDIndex: 4, Arguments: []string{"--model", "synthetic", "delete", "--force", "b1"}, Local: true}
	if err := catalog.ResumeArguments(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	if !launcher.local || launcher.account != "two" || launcher.args[4] != "b111" {
		t.Fatalf("local session launch = %#v", launcher)
	}
}

func TestQueueKeepsOwningProfileAndArgumentForm(t *testing.T) {
	for _, selector := range []string{"", "work", "personal"} {
		catalog, launcher := testCatalog()
		args := []string{"queue", "--thread=b1", "--message", "message"}
		input := sessionmodel.Launch{AccountSelector: selector, SessionSelector: "b1", IDIndex: 1, IDPrefix: "--thread=", Arguments: args}
		err := catalog.ResumeArguments(t.Context(), input)
		if selector == "personal" {
			if err == nil || launcher.account != "" {
				t.Fatal("conflicting account queued a message")
			}
			continue
		}
		if err != nil || launcher.account != "two" || !reflect.DeepEqual(launcher.args, []string{"queue", "--thread=b111", "--message", "message"}) || args[1] != "--thread=b1" {
			t.Fatalf("queue lost owning profile or arguments: %#v, %v", launcher, err)
		}
	}
}

func TestResumeResolvesExactThreadNameAcrossProfiles(t *testing.T) {
	catalog, launcher := testCatalog()
	input := sessionmodel.Launch{
		SessionSelector: "Repair",
		IDIndex:         1,
		Arguments:       []string{"resume", "Repair", "continue"},
	}
	if err := catalog.ResumeArguments(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	if launcher.account != "two" || !reflect.DeepEqual(launcher.args, []string{"resume", "b111", "continue"}) {
		t.Fatalf("name launch = %#v", launcher)
	}

	catalog.reader = readerFake{map[string][]sessionentity.Session{
		"one": {{ID: "a111", ThreadName: "Duplicate", UpdatedUnix: 1, Path: "a"}},
		"two": {{ID: "b111", ThreadName: "Duplicate", UpdatedUnix: 2, Path: "b"}},
	}}
	if _, err := catalog.Resolve(t.Context(), "Duplicate"); err == nil {
		t.Fatal("ambiguous thread name accepted")
	}
}

func TestResumeLastUsesNewestActiveTopLevelSessionInCurrentDirectory(t *testing.T) {
	project := t.TempDir()
	other := t.TempDir()
	t.Chdir(project)
	launcher := &launcherFake{}
	catalog := NewCatalog(accountsFake{}, readerFake{map[string][]sessionentity.Session{
		"one": {
			{ID: "a111", CWD: project, UpdatedUnix: 10, Path: "/profiles/one/sessions/a111.jsonl"},
		},
		"two": {
			{ID: "b555", CWD: project, Source: "exec", UpdatedUnix: 70, Path: "/profiles/two/sessions/b555.jsonl"},
			{ID: "b999", CWD: other, Source: "cli", UpdatedUnix: 50, Path: "/profiles/two/sessions/b999.jsonl"},
			{ID: "b888", CWD: project, UpdatedUnix: 60, Path: "/profiles/two/archived_sessions/b888.jsonl"},
			{ID: "b777", CWD: project, UpdatedUnix: 55, ParentThreadID: "parent", Path: "/profiles/two/sessions/b777.jsonl"},
			{ID: "b666", CWD: project, UpdatedUnix: 40, Path: "/profiles/two/sessions/b666.jsonl"},
		},
	}}, launcher)

	input := sessionmodel.Launch{
		SessionSelector: "--last",
		IDIndex:         1,
		Arguments:       []string{"resume", "--last", "continue"},
	}
	if err := catalog.ResumeArguments(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	if launcher.account != "two" || !reflect.DeepEqual(launcher.args, []string{"resume", "b777", "continue"}) {
		t.Fatalf("default last launch = %#v", launcher)
	}

	launcher.account, launcher.args = "", nil
	input.Arguments = []string{"resume", "--last", "--all", "continue"}
	if err := catalog.ResumeArguments(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	if launcher.account != "two" || !reflect.DeepEqual(launcher.args, []string{"resume", "b777", "--all", "continue"}) {
		t.Fatalf("all last launch = %#v", launcher)
	}

	launcher.account, launcher.args = "", nil
	input.Arguments = []string{"resume", "--last", "--all", "--include-non-interactive", "continue"}
	if err := catalog.ResumeArguments(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	if launcher.account != "two" || !reflect.DeepEqual(launcher.args, []string{"resume", "b555", "--all", "--include-non-interactive", "continue"}) {
		t.Fatalf("non-interactive last launch = %#v", launcher)
	}
}

func TestResumeNameUsesActiveInteractiveDisplayLabelOnly(t *testing.T) {
	launcher := &launcherFake{}
	catalog := NewCatalog(accountsFake{}, readerFake{map[string][]sessionentity.Session{
		"one": {
			{ID: "a100", ThreadName: "Named", Preview: "ignored preview", Source: "cli", UpdatedUnix: 10, Path: "/profiles/one/sessions/a100.jsonl"},
			{ID: "a200", Preview: "Preview label", Source: "vscode", UpdatedUnix: 20, Path: "/profiles/one/sessions/a200.jsonl"},
			{ID: "a300", ThreadName: "Exec label", Source: "exec", UpdatedUnix: 30, Path: "/profiles/one/sessions/a300.jsonl"},
		},
		"two": {
			{ID: "b100", ThreadName: "Archived label", Source: "cli", UpdatedUnix: 40, Path: "/profiles/two/archived_sessions/b100.jsonl"},
			{ID: "b200", ThreadName: "Custom label", Source: "custom", UpdatedUnix: 50, Path: "/profiles/two/sessions/b200.jsonl"},
		},
	}}, launcher)

	for selector, wantID := range map[string]string{"Named": "a100", "Preview label": "a200"} {
		input := sessionmodel.Launch{SessionSelector: selector, IDIndex: 1, Arguments: []string{"resume", selector}}
		if err := catalog.ResumeArguments(t.Context(), input); err != nil {
			t.Fatalf("resume %q: %v", selector, err)
		}
		if !reflect.DeepEqual(launcher.args, []string{"resume", wantID}) {
			t.Fatalf("resume %q args = %#v", selector, launcher.args)
		}
	}
	for _, selector := range []string{"Exec label", "Archived label", "Custom label"} {
		if _, err := catalog.Resolve(t.Context(), selector); err == nil {
			t.Fatalf("non-native-visible name %q resolved", selector)
		}
	}
}
