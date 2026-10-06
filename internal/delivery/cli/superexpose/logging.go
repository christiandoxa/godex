package superexpose

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/redact"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	runtimerepo "github.com/christiandoxa/godex/internal/repository/runtime"
)

type exposeAuditSink interface {
	Append(context.Context, runtimemodel.Event) error
}

type exposeAuditLog struct {
	ctx  context.Context
	sink exposeAuditSink
}

func newExposeAuditLog(ctx context.Context) (*exposeAuditLog, error) {
	home, err := resolveExposeAuditHome()
	if err != nil {
		return nil, err
	}
	return &exposeAuditLog{
		ctx:  context.WithoutCancel(ctx),
		sink: runtimerepo.NewLog(filepath.Join(home, "logs")),
	}, nil
}

func newExposeAuditLogWithSink(ctx context.Context, sink exposeAuditSink) *exposeAuditLog {
	if ctx == nil {
		ctx = context.Background()
	}
	return &exposeAuditLog{ctx: context.WithoutCancel(ctx), sink: sink}
}

func resolveExposeAuditHome() (string, error) {
	value := os.Getenv("GODEX_HOME")
	if value == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve Godex home for expose audit log: %w", err)
		}
		value = filepath.Join(userHome, ".godex")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve Godex home for expose audit log: %w", err)
	}
	clean := filepath.Clean(absolute)
	if clean == filepath.Dir(clean) {
		return "", errors.New("GODEX_HOME must not be the filesystem root")
	}
	return clean, nil
}

func (audit *exposeAuditLog) event(kind string, fields map[string]string) error {
	if audit == nil || audit.sink == nil {
		return nil
	}
	clean := make(map[string]string, len(fields))
	for key, value := range fields {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		clean[key] = redact.Secrets(value)
	}
	return audit.sink.Append(audit.ctx, runtimemodel.Event{
		Kind:    kind,
		Message: exposeAuditMessage(clean),
		Fields:  clean,
	})
}

func (audit *exposeAuditLog) tryEvent(kind string, fields map[string]string) {
	_ = audit.event(kind, fields)
}

func exposeAuditMessage(fields map[string]string) string {
	if len(fields) == 0 {
		return ""
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+fields[key])
	}
	return strings.Join(parts, " ")
}
