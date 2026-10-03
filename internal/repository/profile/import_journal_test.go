package profile

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	"github.com/christiandoxa/godex/internal/helper/fileutil"
)

func TestReplaceAuthCommitsAndCleansItsJournal(t *testing.T) {
	store, profile := profileWithAuth(t, []byte(`{"access_token":"previous"}`))
	if err := store.ReplaceAuth(t.Context(), profile.Name, []byte(`{"access_token":"next"}`)); err != nil {
		t.Fatal(err)
	}
	assertProfileAuth(t, profile.CodexHome, `{"access_token":"next"}`)
	for _, path := range []string{store.importAuthJournalPath(), filepath.Join(profile.CodexHome, profileImportAuthBackupName)} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("transaction file %s remains: %v", filepath.Base(path), err)
		}
	}
}

func TestRepairImportAuthJournalsRecoversCrashPhases(t *testing.T) {
	for _, fixture := range []struct {
		phase        string
		currentAuth  string
		wantAuth     string
		wantRecovery int
	}{
		{phase: "prepared", currentAuth: `{"access_token":"previous"}`, wantAuth: `{"access_token":"previous"}`, wantRecovery: 1},
		{phase: "backed_up", currentAuth: `{"access_token":"next"}`, wantAuth: `{"access_token":"previous"}`, wantRecovery: 1},
		{phase: "committed", currentAuth: `{"access_token":"next"}`, wantAuth: `{"access_token":"next"}`, wantRecovery: 1},
	} {
		t.Run(fixture.phase, func(t *testing.T) {
			store, profile := profileWithAuth(t, []byte(`{"access_token":"previous"}`))
			writeProfileAuthJournalFixture(t, store, profile, fixture.phase, fixture.currentAuth)

			recovered, err := store.RepairImportAuthJournals(t.Context())
			if err != nil || recovered != fixture.wantRecovery {
				t.Fatalf("recovered = %d, err = %v", recovered, err)
			}
			assertProfileAuth(t, profile.CodexHome, fixture.wantAuth)
			if _, err := os.Lstat(filepath.Join(profile.CodexHome, profileImportAuthBackupName)); !os.IsNotExist(err) {
				t.Fatalf("auth backup remains: %v", err)
			}
			if _, err := os.Lstat(store.importAuthJournalPath()); !os.IsNotExist(err) {
				t.Fatalf("auth journal remains: %v", err)
			}
		})
	}
}

func TestProfileReadAutomaticallyRecoversImportedAuth(t *testing.T) {
	store, profile := profileWithAuth(t, []byte(`{"access_token":"previous"}`))
	writeProfileAuthJournalFixture(t, store, profile, "backed_up", `{"access_token":"next"}`)
	if _, err := store.Resolve(t.Context(), profile.Name); err != nil {
		t.Fatal(err)
	}
	assertProfileAuth(t, profile.CodexHome, `{"access_token":"previous"}`)
}

func TestRepairImportAuthJournalRejectsChangedProfileHome(t *testing.T) {
	store, profile := profileWithAuth(t, []byte(`{"access_token":"previous"}`))
	writeProfileAuthJournalFixture(t, store, profile, "backed_up", `{"access_token":"next"}`)
	journal := profileImportAuthJournal{
		Version: profileImportAuthJournalV1, Profile: profile.Name,
		CodexHome:  filepath.Join(t.TempDir(), "outside"),
		BackupName: profileImportAuthBackupName, Phase: "backed_up",
	}
	if _, err := store.writeProfileImportAuthJournal(journal); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RepairImportAuthJournals(t.Context()); err == nil {
		t.Fatal("journal with a changed home was accepted")
	}
	assertProfileAuth(t, profile.CodexHome, `{"access_token":"next"}`)
}

func TestRepairImportAuthJournalWaitsForProfileReaders(t *testing.T) {
	store, profile := profileWithAuth(t, []byte(`{"access_token":"previous"}`))
	release, err := store.Acquire(t.Context(), profile.Name)
	if err != nil {
		t.Fatal(err)
	}
	writeProfileAuthJournalFixture(t, store, profile, "backed_up", `{"access_token":"next"}`)

	if _, err := store.RepairImportAuthJournals(t.Context()); err == nil {
		t.Fatal("recovery mutated a profile held by a reader")
	}
	assertProfileAuth(t, profile.CodexHome, `{"access_token":"next"}`)
	if err := release(); err != nil {
		t.Fatal(err)
	}

	if recovered, err := store.RepairImportAuthJournals(t.Context()); err != nil || recovered != 1 {
		t.Fatalf("recovered = %d, err = %v", recovered, err)
	}
	assertProfileAuth(t, profile.CodexHome, `{"access_token":"previous"}`)
}

func profileWithAuth(t *testing.T, auth []byte) (*Store, profileentity.Profile) {
	t.Helper()
	store := NewStore(t.TempDir())
	profile := profileentity.Profile{
		Name: "work", CodexHome: store.ManagedHome("work"), Managed: true,
		Provider: profileentity.Provider{Kind: profileentity.ProviderOpenAI},
	}
	if err := store.ImportOpenAI(context.Background(), profile, auth, true); err != nil {
		t.Fatal(err)
	}
	return store, profile
}

func writeProfileAuthJournalFixture(t *testing.T, store *Store, profile profileentity.Profile, phase, current string) {
	t.Helper()
	if _, err := fileutil.AtomicWrite(
		filepath.Join(profile.CodexHome, profileImportAuthBackupName),
		[]byte(`{"access_token":"previous"}`),
	); err != nil {
		t.Fatal(err)
	}
	journal := profileImportAuthJournal{
		Version: profileImportAuthJournalV1, Profile: profile.Name,
		CodexHome: profile.CodexHome, BackupName: profileImportAuthBackupName, Phase: phase,
	}
	if _, err := store.writeProfileImportAuthJournal(journal); err != nil {
		t.Fatal(err)
	}
	journalData, err := os.ReadFile(store.importAuthJournalPath())
	if err != nil || strings.Contains(string(journalData), "access_token") || strings.Contains(string(journalData), "previous") {
		t.Fatalf("journal contains credential data: %q, err = %v", journalData, err)
	}
	if _, err := fileutil.AtomicWrite(filepath.Join(profile.CodexHome, profileAuthFileName), []byte(current)); err != nil {
		t.Fatal(err)
	}
}

func assertProfileAuth(t *testing.T, home, want string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(home, profileAuthFileName))
	if err != nil || string(got) != want {
		t.Fatalf("auth = %q, err = %v", got, err)
	}
}
