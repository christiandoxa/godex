package runtime

import (
	"context"
	"os"
	"testing"
)

func TestStateStoreSelectedSavePreservesSectionsAndRecoversAfterRestart(t *testing.T) {
	root := t.TempDir()
	store := NewStateStore(root)
	if err := store.Save(
		context.Background(),
		[]byte(`{"active":"first","bindings":{"response":"a"}}`),
		[]byte(`{"response":"a"}`),
		[]byte(`{"first":{"score":1}}`),
		[]byte(`{"first":{"remaining":80}}`),
		[]byte(`{"first":{"retry_until":10}}`),
	); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSelected(
		context.Background(),
		[]byte(`{"active":"second"}`), nil, []byte(`{"second":{"score":2}}`), nil, nil, 0x09,
	); err != nil {
		t.Fatal(err)
	}

	loaded, err := NewStateStore(root).LoadWithRecovery(context.Background())
	if err != nil || loaded.RecoveredFromBackup {
		t.Fatalf("load after selected save = %#v, %v", loaded, err)
	}
	if got := string(loaded.State); got != `{"active":"second","bindings":{"response":"a"}}` {
		t.Fatalf("selected state = %s", got)
	}
	if got := string(loaded.UsageSnapshots); got != `{"first":{"remaining":80}}` {
		t.Fatalf("unselected usage snapshots = %s", got)
	}

	if err := os.WriteFile(store.StatePath(), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	recovered, err := NewStateStore(root).LoadWithRecovery(context.Background())
	if err != nil || !recovered.RecoveredFromBackup {
		t.Fatalf("recovery = %#v, %v", recovered, err)
	}
	if got := string(recovered.State); got != `{"active":"second","bindings":{"response":"a"}}` {
		t.Fatalf("recovered state = %s", got)
	}
	restarted, err := NewStateStore(root).LoadWithRecovery(context.Background())
	if err != nil || restarted.RecoveredFromBackup {
		t.Fatalf("restart recovery repeated = %#v, %v", restarted, err)
	}
}

func TestStateStoreUsageOnlySavePreservesStateEnvelope(t *testing.T) {
	root := t.TempDir()
	store := NewStateStore(root)
	if err := store.Save(context.Background(), []byte(`{"active":"first"}`), nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSelected(
		context.Background(), nil, nil, nil, []byte(`{"first":{"remaining":80}}`), nil,
		runtimeUsageSnapshotsMask,
	); err != nil {
		t.Fatal(err)
	}
	loaded, err := NewStateStore(root).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(loaded.State) != `{"active":"first"}` || string(loaded.UsageSnapshots) != `{"first":{"remaining":80}}` {
		t.Fatalf("usage-only save lost state: %#v", loaded)
	}
}

func TestStateStoreFaultKeepsLastGoodSnapshot(t *testing.T) {
	root := t.TempDir()
	store := NewStateStore(root)
	if err := store.Save(context.Background(), []byte(`{"generation":"one"}`), nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	t.Setenv(runtimeStateFaultEnv, "1")
	err := store.Save(context.Background(), []byte(`{"generation":"two"}`), nil, nil, nil, nil)
	if err == nil {
		t.Fatalf("fault save error = %v", err)
	}
	loaded, loadErr := NewStateStore(root).Load(context.Background())
	if loadErr != nil || string(loaded.State) != `{"generation":"one"}` {
		t.Fatalf("last-good state = %s, %v", loaded.State, loadErr)
	}
}

func TestStateStoreContinuationFailureStopsStateCommit(t *testing.T) {
	root := t.TempDir()
	store := NewStateStore(root)
	if err := store.Save(context.Background(), []byte(`{"version":"one"}`), []byte(`{"response":"one"}`), nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	t.Setenv(legacyContinuationFaultEnv, "1")
	err := store.Save(context.Background(), []byte(`{"version":"two"}`), []byte(`{"response":"two"}`), nil, nil, nil)
	if err == nil {
		t.Fatal("continuation sidecar fault was ignored")
	}
	loaded, loadErr := NewStateStore(root).Load(context.Background())
	if loadErr != nil || string(loaded.State) != `{"version":"one"}` || string(loaded.Continuations) != `{"response":"one"}` {
		t.Fatalf("state after failed earlier continuation commit = %#v, %v", loaded, loadErr)
	}

	t.Setenv(runtimeStateFaultEnv, "1")
	err = store.Save(context.Background(), []byte(`{"version":"three"}`), []byte(`{"response":"three"}`), nil, nil, nil)
	if err == nil {
		t.Fatal("state sidecar fault was ignored")
	}
	loaded, loadErr = NewStateStore(root).Load(context.Background())
	if loadErr != nil || string(loaded.State) != `{"version":"one"}` || string(loaded.Continuations) != `{"response":"three"}` {
		t.Fatalf("state after continuation-first crash window = %#v, %v", loaded, loadErr)
	}
}

func TestStateStoreSelectedSidecarDoesNotLoadUnselectedContinuation(t *testing.T) {
	root := t.TempDir()
	store := NewStateStore(root)
	if err := store.Save(context.Background(), []byte(`{"state":"ok"}`), []byte(`{"response":"ok"}`), nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(store.ContinuationsLastGoodPath()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.ContinuationsPath(), []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSelected(context.Background(), nil, nil, []byte(`{"score":1}`), nil, nil, 0x08); err != nil {
		t.Fatalf("unrelated selected save failed: %v", err)
	}
	content, err := os.ReadFile(store.StatePath())
	if err != nil || len(content) == 0 {
		t.Fatalf("state after selected save = %q, %v", content, err)
	}
}

func TestStateStoreContinuationOnlySavePreservesRuntimeState(t *testing.T) {
	root := t.TempDir()
	store := NewStateStore(root)
	if err := store.Save(context.Background(), []byte(`{"state":"keep"}`), []byte(`{"old":true}`), nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSelected(context.Background(), nil, []byte(`{"new":true}`), nil, nil, nil, 0x04); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background())
	if err != nil || string(loaded.State) != `{"state":"keep"}` || string(loaded.Continuations) != `{"new":true,"old":true}` {
		t.Fatalf("continuation-only save = %#v, %v", loaded, err)
	}
}

func TestStateStoreUsageOnlySavePreservesRuntimeState(t *testing.T) {
	root := t.TempDir()
	store := NewStateStore(root)
	if err := store.Save(context.Background(), []byte(`{"state":"keep"}`), nil, nil, []byte(`{"old":1}`), nil); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSelected(context.Background(), nil, nil, nil, []byte(`{"new":2}`), nil, 0x10); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background())
	if err != nil || string(loaded.State) != `{"state":"keep"}` || string(loaded.UsageSnapshots) != `{"new":2,"old":1}` {
		t.Fatalf("usage-only save = %#v, %v", loaded, err)
	}
}

func TestStateStoreRejectsUnknownAndConflictingSectionMasks(t *testing.T) {
	store := NewStateStore(t.TempDir())
	for _, mask := range []uint8{0x03, 0x40} {
		if err := store.SaveSelected(context.Background(), []byte(`{}`), nil, nil, nil, nil, mask); err == nil {
			t.Fatalf("invalid section mask %#x accepted", mask)
		}
	}
}

func TestContinuationJournalSaveMergesAndRecovers(t *testing.T) {
	root := t.TempDir()
	store := NewStateStore(root)
	if err := store.SaveContinuationJournal(context.Background(), []byte(`{"response":"a","session":"s"}`), 20); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveContinuationJournal(context.Background(), []byte(`{"response":null,"turn":"t"}`), 10); err != nil {
		t.Fatal(err)
	}
	journal, err := store.LoadContinuationJournal(context.Background())
	if err != nil || journal.SavedAt != 20 || string(journal.Data) != `{"session":"s","turn":"t"}` {
		t.Fatalf("journal = %#v, %v", journal, err)
	}
	if err := os.WriteFile(store.ContinuationJournalPath(), []byte("bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	recovered, err := NewStateStore(root).LoadContinuationJournal(context.Background())
	if err != nil || !recovered.RecoveredFromBackup || string(recovered.Data) != string(journal.Data) {
		t.Fatalf("journal recovery = %#v, %v", recovered, err)
	}
}

func TestContinuationJournalFaultKeepsLastGoodAndSavedAt(t *testing.T) {
	root := t.TempDir()
	store := NewStateStore(root)
	if err := store.SaveContinuationJournal(context.Background(), []byte(`{"response":"one"}`), 20); err != nil {
		t.Fatal(err)
	}
	t.Setenv(runtimeContinuationFaultEnv, "1")
	if err := store.SaveContinuationJournal(context.Background(), []byte(`{"response":"two"}`), 30); err == nil {
		t.Fatal("journal fault was ignored")
	}
	loaded, err := NewStateStore(root).LoadContinuationJournal(context.Background())
	if err != nil || loaded.SavedAt != 20 || string(loaded.Data) != `{"response":"one"}` {
		t.Fatalf("journal after fault = %#v, %v", loaded, err)
	}
}

func TestContinuationJournalDoesNotResurrectReleasedReplay(t *testing.T) {
	root := t.TempDir()
	store := NewStateStore(root)
	if err := store.SaveContinuationJournal(context.Background(), []byte(`{"response":"owner-a"}`), 20); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveContinuationJournal(context.Background(), []byte(`{"response":null}`), 21); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveContinuationJournal(context.Background(), []byte(`{"response":"owner-a"}`), 20); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadContinuationJournal(context.Background())
	if err != nil || string(loaded.Data) != `{}` || loaded.SavedAt != 21 {
		t.Fatalf("released continuation replay = %#v, %v", loaded, err)
	}
}

func TestContinuationJournalReleaseTombstoneBlocksAbsentKeyReplay(t *testing.T) {
	root := t.TempDir()
	store := NewStateStore(root)
	if err := store.SaveContinuationJournal(context.Background(), []byte(`{"response":null}`), 21); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveContinuationJournal(context.Background(), []byte(`{"response":"stale"}`), 20); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadContinuationJournal(context.Background())
	if err != nil || string(loaded.Data) != `{}` || loaded.SavedAt != 21 {
		t.Fatalf("absent-key release replay = %#v, %v", loaded, err)
	}
}

func TestContinuationJournalRecoversPartialPrimaryAndFailsClosedWithTwoBadCopies(t *testing.T) {
	root := t.TempDir()
	store := NewStateStore(root)
	if err := store.SaveContinuationJournal(context.Background(), []byte(`{"response":"safe"}`), 10); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(store.ContinuationJournalLastGoodPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.ContinuationJournalPath(), []byte(`{"generation":`), 0o600); err != nil {
		t.Fatal(err)
	}
	recovered, err := NewStateStore(root).LoadContinuationJournal(context.Background())
	if err != nil || !recovered.RecoveredFromBackup || string(recovered.Data) != `{"response":"safe"}` {
		t.Fatalf("partial journal recovery = %#v, %v", recovered, err)
	}
	repaired, err := os.ReadFile(store.ContinuationJournalPath())
	if err != nil || string(repaired) != string(backup) {
		t.Fatalf("repaired journal = %q, %v", repaired, err)
	}
	if err := os.WriteFile(store.ContinuationJournalPath(), []byte(`{"generation":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.ContinuationJournalLastGoodPath(), []byte(`not-json`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStateStore(root).LoadContinuationJournal(context.Background()); err == nil {
		t.Fatal("corrupt journal copies were treated as empty state")
	}
	primary, primaryErr := os.ReadFile(store.ContinuationJournalPath())
	lastGood, backupErr := os.ReadFile(store.ContinuationJournalLastGoodPath())
	if primaryErr != nil || backupErr != nil || string(primary) != `{"generation":` || string(lastGood) != "not-json" {
		t.Fatalf("corrupt journal copies changed: primary=%q/%v backup=%q/%v", primary, primaryErr, lastGood, backupErr)
	}
}

func TestContinuationJournalLoadsLegacyRawBackup(t *testing.T) {
	root := t.TempDir()
	store := NewStateStore(root)
	legacy := []byte(`{"response":"legacy"}`)
	if err := os.WriteFile(store.ContinuationJournalLastGoodPath(), legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.ContinuationJournalPath(), []byte(`{"generation":`), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := NewStateStore(root).LoadContinuationJournal(context.Background())
	if err != nil || !loaded.RecoveredFromBackup || string(loaded.Data) != string(legacy) {
		t.Fatalf("legacy journal recovery = %#v, %v", loaded, err)
	}
}

func TestContinuationJournalLoadsReferenceVersionedEnvelope(t *testing.T) {
	root := t.TempDir()
	store := NewStateStore(root)
	content := []byte(`{"generation":4,"value":{"saved_at":33,"continuations":{"response":"reference"}}}`)
	if err := os.WriteFile(store.ContinuationJournalPath(), content, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadContinuationJournal(context.Background())
	if err != nil || loaded.Generation != 4 || loaded.SavedAt != 33 || string(loaded.Data) != `{"response":"reference"}` {
		t.Fatalf("reference journal envelope = %#v, %v", loaded, err)
	}
}
