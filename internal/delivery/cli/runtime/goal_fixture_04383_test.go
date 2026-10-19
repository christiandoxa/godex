package runtime

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
)

func openGoalFixtureDB04383(t *testing.T, path string) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := database.Exec("PRAGMA busy_timeout=2000"); err != nil {
		t.Fatal(err)
	}
	return database
}

func TestProdex04383GoalFixtureWriterBoundsReaderContention(t *testing.T) {
	const session = "00000000-0000-4000-8000-000000000001"
	path := fixtureGoalDB04360(t, t.TempDir(), session, "active")
	reader := openGoalFixtureDB04383(t, path)
	transaction, err := reader.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()
	var status string
	if err := transaction.QueryRowContext(t.Context(), "SELECT status FROM thread_goals").Scan(&status); err != nil {
		t.Fatal(err)
	}
	writer := openGoalFixtureDB04383(t, path)
	if err := writer.PingContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	_, err = writer.ExecContext(t.Context(), "UPDATE thread_goals SET status='usage_limited'")
	if err == nil || !strings.Contains(err.Error(), "SQLITE_BUSY") {
		t.Fatalf("locked fixture write error=%v", err)
	}
	if elapsed := time.Since(started); elapsed < time.Second {
		t.Fatalf("fixture writer rejected reader contention immediately: %s", elapsed)
	}
	if err := transaction.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.ExecContext(t.Context(), "UPDATE thread_goals SET status='usage_limited'"); err != nil {
		t.Fatal(err)
	}
	if err := reader.QueryRowContext(t.Context(), "SELECT status FROM thread_goals").Scan(&status); err != nil || status != "usage_limited" {
		t.Fatalf("fixture did not recover after contention: status=%q err=%v", status, err)
	}
}

func TestProdex04383LiveGoalFixturesCancelBeforeTrigger(t *testing.T) {
	for _, name := range []string{"single recovery", "multiple generations"} {
		t.Run(name, func(t *testing.T) {
			marker, err := newSessionStartMarker04360()
			if err != nil {
				t.Fatal(err)
			}
			defer marker.Close()
			started, trigger := make(chan struct{}), make(chan struct{})
			const session = "00000000-0000-4000-8000-000000000001"
			run := (&liveGoalProcess04361{id: session, started: started, trigger: trigger}).Run
			if name == "multiple generations" {
				run = (&liveGoalMultiProcess04374{session: session, started: started, trigger: trigger}).Run
			}
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan struct{})
			var runErr error
			go func() {
				defer close(done)
				runErr = run(ctx, "", []string{marker.path})
			}()
			defer func() {
				cancel()
				close(trigger)
				<-done
			}()
			select {
			case <-started:
			case <-done:
				t.Fatalf("fixture exited before starting: %v", runErr)
			}
			cancel()
			select {
			case <-done:
				if !errors.Is(runErr, context.Canceled) {
					t.Fatalf("fixture cancellation error=%v", runErr)
				}
			case <-time.After(time.Second):
				t.Fatal("fixture ignored cancellation before trigger")
			}
		})
	}
}
