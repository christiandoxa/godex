//go:build !windows

package codex

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"time"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

const (
	nativeRecoveryRetryInterval = 5 * time.Second
	nativeRecoveryPrompt        = "Continue the interrupted task from the persisted session. Preserve completed work and do not repeat completed tool calls."
)

// NativeSessionRecovery controls only the private app-server companion that
// belongs to one managed launch. It is never built from a caller-supplied
// remote endpoint.
type NativeSessionRecovery struct {
	socketPath string
	codexHome  string
	sqliteHome string
	workspace  string
	release    func(context.Context, string) error

	mu          sync.Mutex
	nextAttempt time.Time
}

// Keep the gateway names available to its protocol fixtures while the typed
// result itself is owned by the model boundary.
type NativeRecoveryOutcome = runtimemodel.NativeRecoveryOutcome

const (
	NativeRecoveryDeferred  = runtimemodel.NativeRecoveryDeferred
	NativeRecoveryStopped   = runtimemodel.NativeRecoveryStopped
	NativeRecoveryAccepted  = runtimemodel.NativeRecoveryAccepted
	NativeRecoveryAmbiguous = runtimemodel.NativeRecoveryAmbiguous
)

func newNativeSessionRecovery(
	socketPath, codexHome, sqliteHome, workspace string,
	release func(context.Context, string) error,
) *NativeSessionRecovery {
	return &NativeSessionRecovery{
		socketPath: socketPath,
		codexHome:  codexHome,
		sqliteHome: sqliteHome,
		workspace:  workspace,
		release:    release,
	}
}

func (recovery *NativeSessionRecovery) recover(
	ctx context.Context, sessionID string, expectedGoal bool,
) NativeRecoveryOutcome {
	if recovery == nil || ctx.Err() != nil {
		return NativeRecoveryDeferred
	}
	recovery.mu.Lock()
	if time.Now().Before(recovery.nextAttempt) {
		recovery.mu.Unlock()
		return NativeRecoveryDeferred
	}
	recovery.nextAttempt = time.Now().Add(nativeRecoveryRetryInterval)
	recovery.mu.Unlock()
	if !sessionentity.ValidID(strings.TrimSpace(sessionID)) || strings.TrimSpace(sessionID) != sessionID {
		return NativeRecoveryStopped
	}
	workspace := recovery.workspace
	if persisted := stateDBWorkspaceForSession(recovery.sqliteHome, sessionID); persisted != "" {
		workspace = persisted
	}
	socket, err := connectNativeRecoverySocket(recovery.socketPath)
	if err != nil {
		return NativeRecoveryDeferred
	}
	defer socket.close()
	outcome := recoverNativeSessionOnSocket(
		ctx, socket, recovery.codexHome, workspace, sessionID, expectedGoal,
		func() error {
			if recovery.release == nil {
				return nil
			}
			return recovery.release(ctx, sessionID)
		},
	)
	return outcome
}

// RecoverNativeSession is an optional runtime capability. The typed result
// lets the usecase consume one failure generation while retaining deferred
// attempts for the same owned companion.
func (process *CodexProcess) RecoverNativeSession(
	ctx context.Context, sessionID string, expectedGoal bool,
) runtimemodel.NativeRecoveryOutcome {
	if process == nil {
		return runtimemodel.NativeRecoveryDeferred
	}
	process.nativeRecoveryMu.RLock()
	defer process.nativeRecoveryMu.RUnlock()
	recovery, ok := process.nativeRecovery.(*NativeSessionRecovery)
	if !ok || recovery == nil {
		return runtimemodel.NativeRecoveryDeferred
	}
	return recovery.recover(ctx, sessionID, expectedGoal)
}

func (process *CodexProcess) setNativeSessionRecovery(recovery *NativeSessionRecovery) {
	if process == nil {
		return
	}
	process.nativeRecoveryMu.Lock()
	process.nativeRecovery = recovery
	process.nativeRecoveryMu.Unlock()
}

func (process *CodexProcess) clearNativeSessionRecovery(recovery *NativeSessionRecovery) {
	if process == nil {
		return
	}
	process.nativeRecoveryMu.Lock()
	if process.nativeRecovery == recovery {
		process.nativeRecovery = nil
	}
	process.nativeRecoveryMu.Unlock()
}

func recoverNativeSessionOnSocket(
	ctx context.Context,
	socket *nativeRecoverySocket,
	codexHome, workspace, sessionID string,
	expectedGoal bool,
	release func() error,
) NativeRecoveryOutcome {
	initializeOutcome, initialize := requestNativeRecovery(ctx, socket, 1, "initialize", map[string]any{
		"clientInfo":   map[string]any{"name": "godex-native-recovery", "version": "0.437.7"},
		"capabilities": map[string]any{"experimentalApi": true},
	})
	if initializeOutcome != nativeRecoveryAccepted {
		return NativeRecoveryDeferred
	}
	serverHome, _ := initialize["codexHome"].(string)
	if serverHome == "" || !sameNativePath(serverHome, codexHome) {
		return NativeRecoveryStopped
	}
	initialized, err := json.Marshal(map[string]any{"method": "initialized"})
	if err != nil || socket.write(0x1, initialized, nativeRecoveryDeadline(ctx)) != nil {
		return NativeRecoveryDeferred
	}
	threadOutcome, threadResult := requestNativeRecovery(ctx, socket, 2, "thread/read", map[string]any{
		"threadId": sessionID, "includeTurns": false,
	})
	if threadOutcome != nativeRecoveryAccepted {
		return NativeRecoveryDeferred
	}
	rawThread, exists := threadResult["thread"]
	if !exists {
		return NativeRecoveryDeferred
	}
	thread, validThread := rawThread.(map[string]any)
	if !validThread || thread == nil {
		return NativeRecoveryDeferred
	}
	status, verified := verifiedNativeThreadStatus(thread, workspace, sessionID)
	if !verified {
		return NativeRecoveryStopped
	}
	goalOutcome, goalResult := requestNativeRecovery(ctx, socket, 3, "thread/goal/get", map[string]any{
		"threadId": sessionID,
	})
	if goalOutcome != nativeRecoveryAccepted {
		return NativeRecoveryDeferred
	}
	goalField, exists := goalResult["goal"]
	if !exists {
		return NativeRecoveryDeferred
	}
	var goal map[string]any
	if goalField != nil {
		goal, _ = goalField.(map[string]any)
		if goal == nil {
			return NativeRecoveryDeferred
		}
		if goal["threadId"] != sessionID {
			return NativeRecoveryStopped
		}
	}
	action := nativeRecoveryActionFor(status, expectedGoal, goal, nativeGoalBudgetAvailable(goal))
	switch action {
	case nativeRecoveryDefer:
		return NativeRecoveryDeferred
	case nativeRecoveryStop:
		return NativeRecoveryStopped
	}
	if release != nil {
		if err := release(); err != nil {
			return NativeRecoveryDeferred
		}
	}
	method := "turn/start"
	params := map[string]any{
		"threadId": sessionID,
		"input":    []any{map[string]any{"type": "text", "text": nativeRecoveryPrompt}},
	}
	if action == nativeRecoveryResumeGoal {
		method = "thread/goal/set"
		params = map[string]any{"threadId": sessionID, "status": "active", "origin": "automatic"}
	}
	mutationOutcome, result := requestNativeRecovery(ctx, socket, 4, method, params)
	switch mutationOutcome {
	case nativeRecoveryRejected:
		return NativeRecoveryStopped
	case nativeRecoveryAmbiguous:
		return NativeRecoveryAmbiguous
	case nativeRecoveryAccepted:
		if nativeMutationConfirmed(action, result, sessionID) {
			return NativeRecoveryAccepted
		}
		return NativeRecoveryAmbiguous
	default:
		return NativeRecoveryAmbiguous
	}
}

func nativeRecoveryDeadline(ctx context.Context) time.Time {
	deadline := time.Now().Add(nativeRecoveryRequestTimeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		return contextDeadline
	}
	return deadline
}

type nativeRecoveryAction uint8

const (
	nativeRecoveryDefer nativeRecoveryAction = iota
	nativeRecoveryStop
	nativeRecoveryResumeGoal
	nativeRecoveryContinueTurn
)

type nativeThreadStatus uint8

const (
	nativeThreadUnknown nativeThreadStatus = iota
	nativeThreadIdle
	nativeThreadSystemError
	nativeThreadActive
	nativeThreadNotLoaded
)

func nativeRecoveryActionFor(
	status nativeThreadStatus, expectedGoal bool, goal map[string]any, budgetAvailable bool,
) nativeRecoveryAction {
	if status != nativeThreadIdle && status != nativeThreadSystemError {
		return nativeRecoveryDefer
	}
	if goal == nil {
		if expectedGoal {
			return nativeRecoveryStop
		}
		return nativeRecoveryContinueTurn
	}
	switch goal["status"] {
	case "active", "blocked", "usageLimited":
		if !budgetAvailable {
			return nativeRecoveryStop
		}
		return nativeRecoveryResumeGoal
	default:
		return nativeRecoveryStop
	}
}

func verifiedNativeThreadStatus(thread map[string]any, workspace, sessionID string) (nativeThreadStatus, bool) {
	if thread == nil || thread["id"] != sessionID || thread["sessionId"] != sessionID ||
		thread["ephemeral"] != false || thread["canAcceptDirectInput"] != true {
		return nativeThreadUnknown, false
	}
	cwd, _ := thread["cwd"].(string)
	if cwd == "" || !sameNativePath(cwd, workspace) {
		return nativeThreadUnknown, false
	}
	status, _ := thread["status"].(map[string]any)
	typeName, _ := status["type"].(string)
	switch typeName {
	case "idle":
		return nativeThreadIdle, true
	case "systemError":
		return nativeThreadSystemError, true
	case "active":
		return nativeThreadActive, true
	case "notLoaded":
		return nativeThreadNotLoaded, true
	default:
		return nativeThreadUnknown, true
	}
}

func nativeGoalBudgetAvailable(goal map[string]any) bool {
	if goal == nil || goal["tokenBudget"] == nil {
		return true
	}
	limit, limitOK := nativeInt64(goal["tokenBudget"])
	used, usedOK := nativeInt64(goal["tokensUsed"])
	return limitOK && usedOK && limit > 0 && used >= 0 && used < limit
}

func nativeInt64(value any) (int64, bool) {
	number, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	parsed, err := number.Int64()
	return parsed, err == nil
}

func nativeMutationConfirmed(action nativeRecoveryAction, result map[string]any, sessionID string) bool {
	switch action {
	case nativeRecoveryResumeGoal:
		goal, _ := result["goal"].(map[string]any)
		return goal != nil && goal["threadId"] == sessionID && goal["status"] == "active"
	case nativeRecoveryContinueTurn:
		turn, _ := result["turn"].(map[string]any)
		id, _ := turn["id"].(string)
		return id != ""
	default:
		return false
	}
}

func sameNativePath(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return false
	}
	a, errA := filepath.EvalSymlinks(left)
	b, errB := filepath.EvalSymlinks(right)
	if errA != nil || errB != nil {
		return false
	}
	a, errA = filepath.Abs(a)
	b, errB = filepath.Abs(b)
	return errA == nil && errB == nil && filepath.Clean(a) == filepath.Clean(b)
}
