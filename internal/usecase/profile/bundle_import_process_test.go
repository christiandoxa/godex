package profile

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	profilerepo "github.com/christiandoxa/godex/internal/repository/profile"
)

const bundleRecoveryHelperRoot = "GODEX_TEST_BUNDLE_RECOVERY_ROOT"

func TestBundleJournalRecoverySurvivesProcessRestart(t *testing.T) {
	if root := os.Getenv(bundleRecoveryHelperRoot); root != "" {
		repo := profilerepo.NewStore(root)
		_, err := NewCatalog(repo, &fakeAccounts{}, "").List(context.Background())
		if err != nil {
			_, _ = fmt.Fprintln(os.Stdout, "error:"+err.Error())
			return
		}
		_, _ = fmt.Fprintln(os.Stdout, "recovered")
		return
	}

	root := t.TempDir()
	repo := profilerepo.NewStore(root)
	profile := profileentity.Profile{
		Name: "crashed", CodexHome: repo.ManagedHome("crashed"), Managed: true,
		Provider: profileentity.Provider{Kind: profileentity.ProviderOpenAI},
	}
	const journalID = "0123456789abcdef0123456789abcdef"
	journal := profilemodel.ImportLifecycleJournal{
		Version: 1, ID: journalID, Phase: "preparing",
		Actions: []profilemodel.ImportLifecycleAction{{
			Name: profile.Name, Create: true, After: importLifecycleProfile(profile),
			Files: importLifecycleFiles(profilemodel.ExportedProfile{
				Provider: profilemodel.ProviderSnapshot{Kind: "openai"}, AuthJSON: "synthetic-auth",
			}),
		}},
	}
	if err := repo.WriteBundleImportJournal(journal); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestBundleJournalRecoverySurvivesProcessRestart$")
	command.Env = append(os.Environ(), bundleRecoveryHelperRoot+"="+root)
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	line, readErr := bufio.NewReader(output).ReadString('\n')
	rest, restErr := io.ReadAll(output)
	waitErr := command.Wait()
	if readErr != nil || restErr != nil || waitErr != nil || strings.TrimSpace(line) != "recovered" {
		t.Fatalf("bundle recovery helper = %q%q, read=%v rest=%v wait=%v", line, rest, readErr, restErr, waitErr)
	}
	if journals, err := repo.BundleImportJournals(); err != nil || len(journals) != 0 {
		t.Fatalf("journals after restart = %#v, err=%v", journals, err)
	}
	if _, err := os.Stat(filepath.Join(root, "profiles", "crashed")); !os.IsNotExist(err) {
		t.Fatalf("crashed profile home remains: %v", err)
	}
}
