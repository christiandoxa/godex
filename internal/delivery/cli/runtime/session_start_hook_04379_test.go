package runtime

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

const (
	nativeRoot04379  = "00000000-0000-4000-8000-000000000001"
	nativeChild04379 = "00000000-0000-4000-8000-000000000002"
	nativeNext04379  = "00000000-0000-4000-8000-000000000003"
)

func TestProdex04379SessionStartOwnsLegacyDescendantsAndCursor(t *testing.T) {
	marker, err := newSessionStartMarker04360()
	if err != nil {
		t.Fatal(err)
	}
	defer marker.Close()
	send := func(payload string) {
		t.Helper()
		handled, err := HandleSessionStartNotify04360([]string{sessionStartNotifyCommand04360, marker.path, payload}, nil)
		if err != nil || !handled {
			t.Fatalf("callback %s: handled=%t err=%v", payload, handled, err)
		}
	}
	send(`{"thread-id":"` + nativeChild04379 + `"}`)
	if marker.ID() != nativeChild04379 {
		t.Fatal("legacy-only bootstrap lost")
	}
	send(`{"thread-id":"` + nativeChild04379 + `","session_id":"` + nativeRoot04379 + `"}`)
	if marker.ID() != nativeRoot04379 {
		t.Fatalf("canonical root lost: %q", marker.ID())
	}
	before, err := os.Stat(marker.path)
	if err != nil {
		t.Fatal(err)
	}
	send(`{"thread-id":"` + nativeRoot04379 + `"}`)
	send(`{"session_id":"` + nativeRoot04379 + `"}`)
	send(`{"thread-id":"` + nativeChild04379 + `"}`)
	after, err := os.Stat(marker.path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("repeated callback replaced unread root cursor")
	}
	if marker.ID() != nativeRoot04379 {
		t.Fatalf("legacy descendant hijacked root: %q", marker.ID())
	}
	send(`{"session_id":"` + nativeNext04379 + `"}`)
	if marker.ID() != nativeNext04379 {
		t.Fatal("explicit new SessionStart did not switch root")
	}
	send(`{"thread-id":"` + nativeRoot04379 + `"}`)
	if marker.ID() != nativeNext04379 {
		t.Fatal("old legacy completion redirected new root")
	}
}

func TestProdex04379NativeMarkerRejectsMalformedCanonicalInput(t *testing.T) {
	marker, err := newSessionStartMarker04360()
	if err != nil {
		t.Fatal(err)
	}
	defer marker.Close()
	handled, err := HandleSessionStartNotify04360([]string{
		sessionStartNotifyCommand04360, marker.path,
		`{"session_id":"invalid","thread-id":"` + nativeRoot04379 + `"}`,
	}, nil)
	if !handled || err == nil || marker.ID() != "" {
		t.Fatalf("bad canonical ID fell back to legacy: handled=%t err=%v id=%q", handled, err, marker.ID())
	}
}

func TestProdex04379ConcurrentDescendantNotificationsKeepRoot(t *testing.T) {
	marker, err := newSessionStartMarker04360()
	if err != nil {
		t.Fatal(err)
	}
	defer marker.Close()
	_, err = HandleSessionStartNotify04360([]string{sessionStartNotifyCommand04360, marker.path,
		`{"session_id":"` + nativeRoot04379 + `"}`}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 24)
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			payload := `{"thread-id":"` + nativeChild04379 + `"}`
			if i%2 == 0 {
				payload = `{"session_id":"` + nativeRoot04379 + `"}`
			}
			_, err := HandleSessionStartNotify04360([]string{sessionStartNotifyCommand04360, marker.path, payload}, nil)
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent marker callback: %v", err)
		}
	}
	if marker.ID() != nativeRoot04379 {
		t.Fatalf("concurrent callback changed root: %q", marker.ID())
	}
	if info, err := os.Lstat(marker.path); err != nil || !info.Mode().IsRegular() || info.Size() != 37 {
		t.Fatalf("invalid marker after concurrent writes: %+v %v", info, err)
	}
	if _, err := os.Lstat(filepath.Join(marker.directory, "session.id.lock")); err != nil {
		t.Fatalf("serialized callback has no owned lock: %v", err)
	}
}
