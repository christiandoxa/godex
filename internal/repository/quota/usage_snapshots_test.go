package quota

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestProdex04355UsageSnapshotStoreSurvivesRestartAndRecoversLastGood(t *testing.T) {
	root := t.TempDir()
	store := NewUsageSnapshotStore(root)
	snapshot := quotamodel.UsageSnapshot{
		CheckedAt:      100,
		FiveHourStatus: quotamodel.WindowReady, FiveHourRemainingPercent: 80, FiveHourResetAt: 1000,
		WeeklyStatus: quotamodel.WindowThin, WeeklyRemainingPercent: 20, WeeklyResetAt: 2000,
	}
	if err := store.Save(t.Context(), "account-a", snapshot); err != nil {
		t.Fatal(err)
	}
	restarted := NewUsageSnapshotStore(root)
	got, ok, err := restarted.Load(t.Context(), "account-a")
	if err != nil || !ok || got != snapshot {
		t.Fatalf("restart snapshot = %#v ok=%t err=%v", got, ok, err)
	}
	if err := os.WriteFile(restarted.path(), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, ok, err = NewUsageSnapshotStore(root).Load(t.Context(), "account-a")
	if err != nil || !ok || got != snapshot {
		t.Fatalf("last-good snapshot = %#v ok=%t err=%v", got, ok, err)
	}
}

func TestProdex04355UsageSnapshotStoreLatestCheckedAtWins(t *testing.T) {
	store := NewUsageSnapshotStore(t.TempDir())
	newer := quotamodel.UsageSnapshot{
		CheckedAt:      200,
		FiveHourStatus: quotamodel.WindowReady, FiveHourRemainingPercent: 90, FiveHourResetAt: 1000,
		WeeklyStatus: quotamodel.WindowReady, WeeklyRemainingPercent: 90, WeeklyResetAt: 2000,
	}
	older := newer
	older.CheckedAt = 100
	older.FiveHourRemainingPercent = 10
	if err := store.Save(context.Background(), "account-a", newer); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), "account-a", older); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.Load(context.Background(), "account-a")
	if err != nil || !ok || got != newer {
		t.Fatalf("latest snapshot = %#v ok=%t err=%v", got, ok, err)
	}
}

func TestProdex04355UsageSnapshotStoreRejectsSymlinkRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on some Windows hosts")
	}
	parent := t.TempDir()
	outside := filepath.Join(parent, "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(parent, "linked-root")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	store := NewUsageSnapshotStore(link)
	snapshot := quotamodel.UsageSnapshot{
		CheckedAt:      100,
		FiveHourStatus: quotamodel.WindowReady, FiveHourRemainingPercent: 80, FiveHourResetAt: 1000,
		WeeklyStatus: quotamodel.WindowReady, WeeklyRemainingPercent: 80, WeeklyResetAt: 2000,
	}
	if err := store.Save(t.Context(), "account-a", snapshot); err == nil {
		t.Fatal("symlink snapshot root was accepted")
	}
}
