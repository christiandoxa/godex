package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
	"github.com/christiandoxa/godex/internal/helper/fileutil"
	"github.com/christiandoxa/godex/internal/helper/lockfile"
)

// A canonical SessionStart session_id identifies the root agent tree; inherited
// notify thread-id identifies only the concrete completing descendant.
// Repeated callbacks must never reset the monitor's unread rollout cursor.
func writeSessionStartMarker04379(path string, fields map[string]any) error {
	raw, canonical := fields["session_id"]
	if !canonical {
		raw = fields["thread-id"]
	}
	id, ok := raw.(string)
	if !ok || !sessionentity.ValidID(id) {
		return errors.New("invalid callback session identity")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	release, err := lockfile.Acquire(ctx, path+".lock")
	if err != nil {
		return fmt.Errorf("lock session callback marker: %w", err)
	}
	defer release()

	current, err := readSessionStartMarker04379(path)
	if err != nil {
		return err
	}
	// Legacy notify may bootstrap an empty marker, but may not take over an
	// established root. A repeated root callback is always a no-op.
	if current != "" && (!canonical || strings.EqualFold(current, id)) {
		return nil
	}
	offsetPath := sessionStartOffsetPath04379(path)
	rolloutPath := sessionStartRolloutSidecarPath04379(path)
	transcript, _ := fields["transcript_path"].(string)
	transcript = filepath.Clean(transcript)
	offset := sessionStartTranscriptOffset04379(fields, id)
	if offset >= 0 && len(transcript) <= sessionStartRolloutPathLimit04379 &&
		!strings.ContainsAny(transcript, "\r\n") {
		if _, err := fileutil.AtomicWrite(rolloutPath, []byte(transcript+"\n")); err != nil {
			return fmt.Errorf("publish session callback rollout: %w", err)
		}
		if _, err := fileutil.AtomicWrite(offsetPath, []byte(fmt.Sprintf("%d\n", offset))); err != nil {
			return fmt.Errorf("publish session callback offset: %w", err)
		}
	} else {
		for _, sidecar := range []string{offsetPath, rolloutPath} {
			if err := os.Remove(sidecar); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("clear stale session callback evidence: %w", err)
			}
		}
	}
	if _, err := fileutil.AtomicWrite(path, []byte(id+"\n")); err != nil {
		return fmt.Errorf("publish session callback marker: %w", err)
	}
	return nil
}

func readSessionStartMarker04379(path string) (string, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect session callback marker: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > 64 {
		return "", errors.New("session callback marker is not a bounded regular file")
	}
	body, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("read session callback marker: %w", err)
	}
	value := strings.TrimSpace(string(body))
	if !sessionentity.ValidID(value) {
		return "", nil
	}
	return value, nil
}
