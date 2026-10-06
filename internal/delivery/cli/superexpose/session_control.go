package superexpose

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	sessionPromptMaxMessageBytes = 64 * 1024
	sessionResolutionTimeout     = 15 * time.Second
	sessionResolutionPoll        = 50 * time.Millisecond
	processAncestryLimit         = 64
)

type sessionControlError string

const (
	sessionNoSession                 sessionControlError = "no_session"
	sessionAmbiguousSession          sessionControlError = "ambiguous_session"
	sessionNoCodexWriter             sessionControlError = "no_codex_writer"
	sessionAmbiguousCodexWriter      sessionControlError = "ambiguous_codex_writer"
	sessionThreadIdentityUnavailable sessionControlError = "thread_identity_unavailable"
	sessionThreadIdentityConflict    sessionControlError = "thread_identity_conflict"
	sessionQueueDBUnavailable        sessionControlError = "queue_db_unavailable"
	sessionNotQueueAddressable       sessionControlError = "session_not_queue_addressable"
	sessionTargetEnvUnavailable      sessionControlError = "target_environment_unavailable"
	sessionQueueUnsupported          sessionControlError = "queue_unsupported"
	sessionStaleTarget               sessionControlError = "stale_target"
	sessionQueueFailed               sessionControlError = "queue_failed"
	sessionWriteAmbiguous            sessionControlError = "write_ambiguous"
	sessionVerificationInconclusive  sessionControlError = "verification_inconclusive"
	sessionOutputSourceUnavailable   sessionControlError = "output_source_unavailable"
	sessionOutputSourceAmbiguous     sessionControlError = "output_source_ambiguous"
	sessionOutputSourceChanged       sessionControlError = "output_source_changed"
	sessionInvalidCursor             sessionControlError = "invalid_cursor"
	sessionStaleCursor               sessionControlError = "stale_cursor"
	sessionRecoveryFailed            sessionControlError = "recovery_failed"
	sessionOutputReadFailed          sessionControlError = "output_read_failed"
)

func (err sessionControlError) Error() string { return string(err) }

type processState uint8

const (
	processDead processState = iota
	processRunning
	processStopped
	processZombie
)

func (state processState) live() bool {
	return state == processRunning
}

type processRecord struct {
	pid           uint32
	parentPID     uint32
	uid           uint32
	state         processState
	executable    string
	argv          []string
	cwd           string
	startTime     string
	birthIdentity string
}

type openProcessFile struct {
	path string
}

type targetEnvironment struct {
	home            string
	codexHome       string
	codexSQLiteHome string
	pwd             string
}

type processDetails struct {
	record      processRecord
	environment targetEnvironment
	openFiles   []openProcessFile
}

type resolvedSessionTarget struct {
	godex          processRecord
	writer         processRecord
	threadID       string
	environment    targetEnvironment
	queueDB        string
	stateDB        string
	remoteEndpoint string
	openFiles      []openProcessFile
}

type sessionProcessInspector interface {
	currentUID() (uint32, error)
	list() ([]processRecord, error)
	inspect(uint32) (*processDetails, error)
}

type queueRequestOutcome uint8

const (
	queueRejected queueRequestOutcome = iota
	queuePreflight
	queueAccepted
	queueAmbiguous
)

type queueInvocation struct {
	outcome      queueRequestOutcome
	exitCode     *int
	messageID    string
	submissionID string
	queued       bool
}

type queuePreemptResult struct {
	currentTurnID          string
	currentTurnInterrupted bool
	cancelledSubmissionIDs []string
	remainingSubmissionIDs []string
	queueEmptyAtBoundary   bool
	sessionReady           bool
}

type sessionQueueControl interface {
	checkCapability(resolvedSessionTarget) error
	persistedThread(string, string) (bool, error)
	rolloutPath(string, string) (string, error)
	queueOnce(resolvedSessionTarget, string) queueInvocation
	preempt(resolvedSessionTarget) (queuePreemptResult, error)
	loadedThreadAddressable(resolvedSessionTarget) (bool, error)
}

type sessionBinding struct {
	target   resolvedSessionTarget
	sourceID string
}

type existingSessionService struct {
	process sessionProcessInspector
	queue   sessionQueueControl

	mu         sync.Mutex
	bindings   map[string]sessionBinding
	operation  sync.Mutex
	generation atomic.Uint64
}

func newExistingSessionService(process sessionProcessInspector, queue sessionQueueControl) *existingSessionService {
	return &existingSessionService{
		process: process, queue: queue, bindings: map[string]sessionBinding{},
	}
}

func (service *existingSessionService) binding(key string) (*sessionBinding, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	value, ok := service.bindings[key]
	if !ok {
		return nil, nil
	}
	copy := value
	return &copy, nil
}

func (service *existingSessionService) rememberBinding(key string, target resolvedSessionTarget, sourceID string) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	if existing, ok := service.bindings[key]; ok {
		if !sameProcessIdentity(existing.target.godex, target.godex) ||
			!sameProcessIdentity(existing.target.writer, target.writer) ||
			existing.target.threadID != target.threadID {
			return sessionStaleTarget
		}
		if sourceID == "" {
			sourceID = existing.sourceID
		}
	}
	service.bindings[key] = sessionBinding{target: target, sourceID: sourceID}
	return nil
}

func (service *existingSessionService) resolveSessionTarget(
	workspaceRoot string,
	requestedPID *uint32,
	requestedThreadID string,
	binding *sessionBinding,
) (resolvedSessionTarget, error) {
	if binding != nil && requestedPID == nil {
		value := binding.target.godex.pid
		requestedPID = &value
	}
	deadline := time.Now().Add(sessionResolutionTimeout)
	for {
		target, err := service.resolveTargetForRequest(
			workspaceRoot, requestedPID, requestedThreadID, binding != nil,
		)
		if err == nil {
			if err := service.verifyBinding(binding, target); err != nil {
				return resolvedSessionTarget{}, err
			}
			if requestedThreadID != "" && requestedThreadID != target.threadID {
				return resolvedSessionTarget{}, sessionStaleTarget
			}
			if binding != nil && binding.sourceID != "" {
				path, err := service.outputSource(target)
				if err != nil {
					return resolvedSessionTarget{}, err
				}
				sourceID, err := outputSourceID(path, target.threadID)
				if err != nil || sourceID != binding.sourceID {
					return resolvedSessionTarget{}, sessionStaleTarget
				}
			}
			if err := service.queue.checkCapability(target); err != nil {
				return resolvedSessionTarget{}, sessionQueueUnsupported
			}
			return target, nil
		}
		err = normalizeSessionResolutionError(err, binding, requestedPID, requestedThreadID)
		if !sessionResolutionRetryable(err) || !time.Now().Before(deadline) {
			return resolvedSessionTarget{}, err
		}
		time.Sleep(sessionResolutionPoll)
	}
}

func normalizeSessionResolutionError(err error, binding *sessionBinding, requestedPID *uint32, threadID string) error {
	if errors.Is(err, sessionNoSession) && (binding != nil || requestedPID != nil || threadID != "") {
		return sessionStaleTarget
	}
	return err
}

func sessionResolutionRetryable(err error) bool {
	return errors.Is(err, sessionNoCodexWriter) ||
		errors.Is(err, sessionThreadIdentityUnavailable) ||
		errors.Is(err, sessionNotQueueAddressable) ||
		errors.Is(err, sessionTargetEnvUnavailable) ||
		errors.Is(err, sessionVerificationInconclusive)
}

func (service *existingSessionService) resolveTargetForRequest(
	workspaceRoot string,
	requestedPID *uint32,
	threadID string,
	alreadyNarrowed bool,
) (resolvedSessionTarget, error) {
	if !alreadyNarrowed && requestedPID == nil && threadID != "" {
		return service.resolveTargetForThread(workspaceRoot, threadID)
	}
	target, err := service.resolveTarget(workspaceRoot, requestedPID)
	if err != nil {
		return resolvedSessionTarget{}, err
	}
	return service.resolveWriter(target, workspaceRoot)
}

func (service *existingSessionService) resolveTargetForThread(workspaceRoot, threadID string) (resolvedSessionTarget, error) {
	candidates, err := service.sessionCandidates(workspaceRoot, nil)
	if err != nil {
		return resolvedSessionTarget{}, err
	}
	var matches []resolvedSessionTarget
	var retryable, definitive error
	for _, candidate := range candidates {
		target, err := service.resolveWriter(candidate, workspaceRoot)
		switch {
		case err == nil && target.threadID == threadID:
			matches = append(matches, target)
		case err == nil:
		case sessionResolutionRetryable(err):
			if retryable == nil {
				retryable = err
			}
		default:
			if definitive == nil {
				definitive = err
			}
		}
	}
	if len(matches) == 1 && retryable == nil && definitive == nil {
		return matches[0], nil
	}
	if len(matches) == 0 {
		if retryable != nil {
			return resolvedSessionTarget{}, retryable
		}
		if definitive != nil {
			return resolvedSessionTarget{}, definitive
		}
		return resolvedSessionTarget{}, sessionStaleTarget
	}
	if retryable != nil {
		return resolvedSessionTarget{}, retryable
	}
	if definitive != nil {
		return resolvedSessionTarget{}, definitive
	}
	return resolvedSessionTarget{}, sessionAmbiguousSession
}

func (service *existingSessionService) resolveTarget(workspaceRoot string, requestedPID *uint32) (processRecord, error) {
	candidates, err := service.sessionCandidates(workspaceRoot, requestedPID)
	if err != nil {
		return processRecord{}, err
	}
	if len(candidates) != 1 {
		return processRecord{}, sessionAmbiguousSession
	}
	if candidates[0].birthIdentity == "" {
		return processRecord{}, sessionVerificationInconclusive
	}
	return candidates[0], nil
}

func (service *existingSessionService) sessionCandidates(workspaceRoot string, requestedPID *uint32) ([]processRecord, error) {
	root, err := filepath.EvalSymlinks(workspaceRoot)
	if err != nil {
		return nil, sessionVerificationInconclusive
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, sessionVerificationInconclusive
	}
	uid, err := service.process.currentUID()
	if err != nil {
		return nil, sessionVerificationInconclusive
	}
	processes, err := service.process.list()
	if err != nil {
		return nil, sessionVerificationInconclusive
	}
	candidates := make([]processRecord, 0)
	for _, process := range processes {
		if process.uid != uid || !process.state.live() || !sameCanonicalPath(process.cwd, root) ||
			!plainGodexSession(process) {
			continue
		}
		if requestedPID != nil && process.pid != *requestedPID {
			continue
		}
		candidates = append(candidates, process)
	}
	if len(candidates) == 0 {
		return nil, sessionNoSession
	}
	return candidates, nil
}

func (service *existingSessionService) resolveWriter(godex processRecord, workspaceRoot string) (resolvedSessionTarget, error) {
	processes, err := service.process.list()
	if err != nil {
		return resolvedSessionTarget{}, sessionVerificationInconclusive
	}
	byPID := make(map[uint32]processRecord, len(processes))
	for _, process := range processes {
		byPID[process.pid] = process
	}
	writers := make([]processRecord, 0)
	for _, process := range processes {
		if process.uid != godex.uid || !process.state.live() || process.pid == godex.pid ||
			!sameCanonicalPath(process.cwd, workspaceRoot) || !codexWriter(process) ||
			!processDescendantOf(process.pid, godex.pid, byPID) {
			continue
		}
		writers = append(writers, process)
	}
	switch len(writers) {
	case 0:
		return resolvedSessionTarget{}, sessionNoCodexWriter
	case 1:
	default:
		return resolvedSessionTarget{}, sessionAmbiguousCodexWriter
	}
	if godex.birthIdentity == "" || writers[0].birthIdentity == "" {
		return resolvedSessionTarget{}, sessionVerificationInconclusive
	}
	details, err := service.process.inspect(writers[0].pid)
	if err != nil || details == nil {
		return resolvedSessionTarget{}, sessionTargetEnvUnavailable
	}
	if !sameProcessIdentity(writers[0], details.record) {
		return resolvedSessionTarget{}, sessionStaleTarget
	}
	threadID, err := resolveThreadIdentity(details.openFiles)
	if err != nil {
		return resolvedSessionTarget{}, err
	}
	queueDB, err := exactOpenDatabase(details.openFiles, "queue")
	if err != nil {
		return resolvedSessionTarget{}, err
	}
	if queueDB == "" {
		return resolvedSessionTarget{}, sessionQueueDBUnavailable
	}
	stateDB, err := exactOpenDatabase(details.openFiles, "state")
	if err != nil {
		return resolvedSessionTarget{}, err
	}
	if stateDB == "" {
		return resolvedSessionTarget{}, sessionNotQueueAddressable
	}
	environment, err := validatedTargetEnvironment(details.environment, workspaceRoot)
	if err != nil {
		return resolvedSessionTarget{}, err
	}
	expectedQueue, err := filepath.EvalSymlinks(filepath.Join(environment.codexSQLiteHome, "queue_1.sqlite"))
	if err != nil || filepath.Clean(expectedQueue) != filepath.Clean(queueDB) {
		return resolvedSessionTarget{}, sessionQueueDBUnavailable
	}
	target := resolvedSessionTarget{
		godex: godex, writer: details.record, threadID: threadID,
		environment: environment, queueDB: queueDB, stateDB: stateDB,
		openFiles: append([]openProcessFile(nil), details.openFiles...),
	}
	target.remoteEndpoint = validatedRemoteEndpoint(target)
	addressable, err := service.targetSessionAddressable(target)
	if err != nil {
		return resolvedSessionTarget{}, err
	}
	if !addressable {
		return resolvedSessionTarget{}, sessionNotQueueAddressable
	}
	return target, nil
}

func validatedTargetEnvironment(environment targetEnvironment, workspaceRoot string) (targetEnvironment, error) {
	for _, value := range []string{environment.home, environment.codexHome, environment.codexSQLiteHome, environment.pwd} {
		if value == "" || strings.IndexByte(value, 0) >= 0 {
			return targetEnvironment{}, sessionTargetEnvUnavailable
		}
	}
	if !filepath.IsAbs(environment.codexHome) || !filepath.IsAbs(environment.codexSQLiteHome) {
		return targetEnvironment{}, sessionTargetEnvUnavailable
	}
	pwd, err := filepath.EvalSymlinks(environment.pwd)
	if err != nil || !sameCanonicalPath(pwd, workspaceRoot) {
		return targetEnvironment{}, sessionTargetEnvUnavailable
	}
	return environment, nil
}

func (service *existingSessionService) targetSessionAddressable(target resolvedSessionTarget) (bool, error) {
	if firstCodexPositionalArg(target.writer.argv) == "app-server" {
		return service.queue.loadedThreadAddressable(target)
	}
	persisted, err := service.queue.persistedThread(target.stateDB, target.threadID)
	if err != nil {
		return false, err
	}
	if persisted {
		return true, nil
	}
	return service.queue.loadedThreadAddressable(target)
}

func (service *existingSessionService) revalidate(target resolvedSessionTarget, workspaceRoot string) (resolvedSessionTarget, error) {
	current, err := service.resolveTargetForRequest(workspaceRoot, &target.godex.pid, target.threadID, true)
	if err != nil {
		return resolvedSessionTarget{}, sessionStaleTarget
	}
	if !sameResolvedTarget(target, current) {
		return resolvedSessionTarget{}, sessionStaleTarget
	}
	return current, nil
}

func (service *existingSessionService) verifyBinding(binding *sessionBinding, target resolvedSessionTarget) error {
	if binding == nil {
		return nil
	}
	if !sameResolvedTarget(binding.target, target) {
		return sessionStaleTarget
	}
	return nil
}

func sameResolvedTarget(left, right resolvedSessionTarget) bool {
	return sameProcessIdentity(left.godex, right.godex) &&
		sameProcessIdentity(left.writer, right.writer) &&
		left.godex.uid == right.godex.uid &&
		left.threadID == right.threadID &&
		left.environment == right.environment &&
		left.queueDB == right.queueDB &&
		left.stateDB == right.stateDB &&
		left.remoteEndpoint == right.remoteEndpoint
}

func sameProcessIdentity(left, right processRecord) bool {
	if left.pid != right.pid || left.parentPID != right.parentPID || left.uid != right.uid ||
		left.executable != right.executable || left.cwd != right.cwd ||
		left.startTime != right.startTime || left.birthIdentity != right.birthIdentity ||
		len(left.argv) != len(right.argv) {
		return false
	}
	for index := range left.argv {
		if left.argv[index] != right.argv[index] {
			return false
		}
	}
	return true
}

func processDescendantOf(pid, ancestor uint32, byPID map[uint32]processRecord) bool {
	current := pid
	for range processAncestryLimit {
		process, ok := byPID[current]
		if !ok || !process.state.live() || process.birthIdentity == "" {
			return false
		}
		if process.parentPID == ancestor {
			parent, ok := byPID[ancestor]
			return ok && parent.state.live() && parent.birthIdentity != "" && parent.uid == process.uid
		}
		if process.parentPID == current {
			return false
		}
		parent, ok := byPID[process.parentPID]
		if !ok || parent.uid != process.uid {
			return false
		}
		current = process.parentPID
	}
	return false
}

func plainGodexSession(process processRecord) bool {
	if !strings.EqualFold(filepath.Base(process.executable), executableName("godex")) ||
		len(process.argv) < 2 || process.argv[1] != "s" {
		return false
	}
	for _, arg := range process.argv[2:] {
		if arg == "expose" || arg == "super" || arg == "exec" || arg == "review" ||
			strings.HasPrefix(arg, "godex_super_") || strings.HasPrefix(arg, "prodex_super_") ||
			strings.Contains(arg, "__sub-agent") ||
			strings.Contains(arg, "release-smoke") || strings.Contains(arg, "release_smoke") {
			return false
		}
	}
	return true
}

func codexWriter(process processRecord) bool {
	if !strings.EqualFold(filepath.Base(process.executable), executableName("codex")) {
		return false
	}
	if hasRemoteArgument(process.argv) {
		return false
	}
	command := firstCodexPositionalArg(process.argv)
	return command == "" || command == "resume" || command == "app-server"
}

func executableName(value string) string {
	if strings.HasSuffix(strings.ToLower(value), ".exe") {
		return strings.TrimSuffix(value, filepath.Ext(value))
	}
	return value
}

func firstCodexPositionalArg(argv []string) string {
	for index := 1; index < len(argv); index++ {
		arg := argv[index]
		if arg == "--" {
			return ""
		}
		if strings.HasPrefix(arg, "-") {
			if codexOptionTakesValue(arg) && !strings.Contains(arg, "=") {
				index++
			}
			continue
		}
		return arg
	}
	return ""
}

func codexOptionTakesValue(arg string) bool {
	switch arg {
	case "-c", "--config", "-i", "--image", "-m", "--model", "-p", "--profile",
		"-s", "--sandbox", "-C", "--cd", "--add-dir", "-a", "--ask-for-approval",
		"--remote", "--remote-auth-token-env", "--thread-source":
		return true
	default:
		return false
	}
}

func hasRemoteArgument(argv []string) bool {
	for _, arg := range argv {
		if arg == "--remote" || strings.HasPrefix(arg, "--remote=") {
			return true
		}
	}
	return false
}

func resolveThreadIdentity(files []openProcessFile) (string, error) {
	modern := map[string]bool{}
	legacy := map[string]bool{}
	for _, file := range files {
		if id := modernThreadID(file.path); id != "" {
			modern[id] = true
		}
		if id := legacyThreadID(file.path); id != "" {
			legacy[id] = true
		}
	}
	if len(modern) > 1 || len(legacy) > 1 {
		return "", sessionThreadIdentityConflict
	}
	var modernID, legacyID string
	for id := range modern {
		modernID = id
	}
	for id := range legacy {
		legacyID = id
	}
	if modernID != "" && legacyID != "" && modernID != legacyID {
		return "", sessionThreadIdentityConflict
	}
	if modernID != "" {
		return modernID, nil
	}
	if legacyID != "" {
		return legacyID, nil
	}
	return "", sessionThreadIdentityUnavailable
}

func modernThreadID(path string) string {
	if filepath.Base(filepath.Dir(path)) != "thread-writer-locks" {
		return ""
	}
	name := strings.TrimSuffix(filepath.Base(path), ".lock")
	if name == filepath.Base(path) || !canonicalUUID(name) {
		return ""
	}
	return strings.ToLower(name)
}

func legacyThreadID(path string) string {
	name := filepath.Base(path)
	if !rolloutFileName(name) {
		return ""
	}
	name = strings.TrimSuffix(strings.TrimSuffix(name, ".zst"), ".jsonl")
	parts := strings.Split(name, "-")
	found := ""
	for index := 0; index+5 <= len(parts); index++ {
		candidate := strings.Join(parts[index:index+5], "-")
		if canonicalUUID(candidate) {
			if found != "" {
				return ""
			}
			found = strings.ToLower(candidate)
		}
	}
	return found
}

func canonicalUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	for index, current := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			continue
		}
		if !(current >= '0' && current <= '9') && !(current >= 'a' && current <= 'f') &&
			!(current >= 'A' && current <= 'F') {
			return false
		}
	}
	return true
}

func rolloutFileName(name string) bool {
	return strings.HasPrefix(name, "rollout-") &&
		(strings.HasSuffix(name, ".jsonl") || strings.HasSuffix(name, ".jsonl.zst"))
}

func exactOpenDatabase(files []openProcessFile, kind string) (string, error) {
	paths := map[string]bool{}
	for _, file := range files {
		name := filepath.Base(file.path)
		matches := kind == "queue" && name == "queue_1.sqlite" ||
			kind == "state" && strings.HasPrefix(name, "state_") && strings.HasSuffix(name, ".sqlite")
		if !matches {
			continue
		}
		if strings.HasSuffix(file.path, " (deleted)") {
			return "", sessionQueueDBUnavailable
		}
		path, err := filepath.EvalSymlinks(file.path)
		if err != nil {
			return "", sessionQueueDBUnavailable
		}
		path, err = filepath.Abs(path)
		if err != nil {
			return "", sessionQueueDBUnavailable
		}
		paths[path] = true
	}
	if len(paths) > 1 {
		return "", sessionQueueDBUnavailable
	}
	for path := range paths {
		return path, nil
	}
	return "", nil
}

func sameCanonicalPath(left, right string) bool {
	a, errA := filepath.EvalSymlinks(left)
	b, errB := filepath.EvalSymlinks(right)
	if errA != nil || errB != nil {
		return false
	}
	a, errA = filepath.Abs(a)
	b, errB = filepath.Abs(b)
	return errA == nil && errB == nil && filepath.Clean(a) == filepath.Clean(b)
}

func normalizedThreadID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if !canonicalUUID(value) {
		return "", sessionThreadIdentityConflict
	}
	return strings.ToLower(value), nil
}

func optionalPID(arguments map[string]any) (*uint32, error) {
	value, exists := arguments["godex_pid"]
	if !exists || value == nil {
		if legacy, ok := arguments["prodex_pid"]; ok {
			value, exists = legacy, true
		}
	}
	if !exists || value == nil {
		return nil, nil
	}
	var parsed uint64
	switch typed := value.(type) {
	case float64:
		if typed <= 0 || typed != float64(uint64(typed)) {
			return nil, errors.New("godex_pid must be a positive process id")
		}
		parsed = uint64(typed)
	default:
		return nil, errors.New("godex_pid must be a positive process id")
	}
	if parsed == 0 || parsed > uint64(^uint32(0)) {
		return nil, errors.New("godex_pid must be a positive process id")
	}
	result := uint32(parsed)
	return &result, nil
}

func sessionBindingKey(instanceID string, pid *uint32, threadID string) string {
	pidText := "-"
	if pid != nil {
		pidText = fmt.Sprint(*pid)
	}
	if threadID == "" {
		threadID = "-"
	}
	return instanceID + ":" + pidText + ":" + threadID
}

func validatedRemoteEndpoint(target resolvedSessionTarget) string {
	if firstCodexPositionalArg(target.writer.argv) != "app-server" {
		return ""
	}
	value := ""
	for index := 1; index < len(target.writer.argv); index++ {
		arg := target.writer.argv[index]
		if arg == "--listen" && index+1 < len(target.writer.argv) {
			value = target.writer.argv[index+1]
			break
		}
		if strings.HasPrefix(arg, "--listen=") {
			value = strings.TrimPrefix(arg, "--listen=")
			break
		}
	}
	if value == "" {
		return ""
	}
	raw := strings.TrimPrefix(value, "unix://")
	if raw == value {
		return ""
	}
	if raw == "" {
		raw = filepath.Join(target.environment.codexHome, "app-server-control", "app-server-control.sock")
	}
	if !filepath.IsAbs(raw) || strings.Contains(raw, "..") {
		return ""
	}
	path := filepath.Clean(raw)
	if !strings.HasPrefix(path, filepath.Clean(target.environment.codexHome)+string(filepath.Separator)) {
		return ""
	}
	for _, file := range target.openFiles {
		if isControlSocket(file.path) && sameCanonicalPath(file.path, path) {
			return "unix://" + path
		}
	}
	return ""
}

func isControlSocket(path string) bool {
	name := filepath.Base(path)
	return name == ".s" || name == ".godex-session.sock" || name == ".prodex-session.sock" ||
		(strings.HasSuffix(name, ".sock") && filepath.Base(filepath.Dir(path)) == "app-server-control")
}
