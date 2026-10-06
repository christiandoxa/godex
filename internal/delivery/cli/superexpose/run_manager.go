package superexpose

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/christiandoxa/godex/internal/helper/redact"
)

const (
	maxActiveRuns           = 4
	maxQueuedRuns           = 16
	maxRetainedTerminalRuns = 32
	runOutputMaxBytes       = 256 * 1024
	maxRunEvents            = 256
	maxRunEventTextBytes    = 8 * 1024
	maxEventPage            = 64
)

type runState string

const (
	runQueued      runState = "queued"
	runStarting    runState = "starting"
	runRunning     runState = "running"
	runSucceeded   runState = "succeeded"
	runFailed      runState = "failed"
	runCancelled   runState = "cancelled"
	runStartFailed runState = "start_failed"
)

func (state runState) terminal() bool {
	switch state {
	case runSucceeded, runFailed, runCancelled, runStartFailed:
		return true
	default:
		return false
	}
}

type runEvent struct {
	Seq  uint64
	Type string
	Text string
}

type runRecord struct {
	state           runState
	createdAt       uint64
	startedAt       *uint64
	finishedAt      *uint64
	exitStatus      *int
	output          string
	outputTruncated bool
	events          []runEvent
	nextSeq         uint64
	cancelRequested bool
	child           *exec.Cmd
	provider        string
	model           string
	reasoningEffort string
}

type queuedRun struct {
	id     string
	task   string
	args   []string
	apiEnv map[string]string
}

type runManager struct {
	workspace     string
	baseArgs      []string
	instanceID    string
	workspaceName string
	executable    string

	mu           sync.Mutex
	runs         map[string]*runRecord
	queue        []queuedRun
	active       int
	shuttingDown bool
	workers      sync.WaitGroup
}

func newRunManager(workspace string, baseArgs []string, instanceID, workspaceName string) (*runManager, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve current Godex executable: %w", err)
	}
	return &runManager{
		workspace: workspace, baseArgs: append([]string(nil), baseArgs...),
		instanceID: instanceID, workspaceName: workspaceName, executable: executable,
		runs: make(map[string]*runRecord),
	}, nil
}

func (manager *runManager) start(arguments map[string]any) (map[string]any, error) {
	task, err := requiredBoundedString(arguments, "task", 65_536)
	if err != nil {
		return nil, err
	}
	childArgs, metadata, apiEnv, err := manager.buildChildArgs(arguments)
	if err != nil {
		return nil, err
	}
	runID, err := newRunID()
	if err != nil {
		return nil, err
	}
	record := &runRecord{
		state: runQueued, createdAt: nowMillis(),
		provider: metadata.provider, model: metadata.model, reasoningEffort: metadata.reasoning,
	}
	pushRunEvent(record, "run_queued", "")

	manager.mu.Lock()
	if manager.shuttingDown {
		manager.mu.Unlock()
		return nil, errors.New("run manager is stopping")
	}
	if len(manager.queue) >= maxQueuedRuns && manager.active >= maxActiveRuns {
		manager.mu.Unlock()
		return nil, errors.New("run queue is full")
	}
	manager.runs[runID] = record
	manager.queue = append(manager.queue, queuedRun{id: runID, task: task, args: childArgs, apiEnv: apiEnv})
	manager.dispatchLocked()
	result := manager.summaryLocked(runID, record)
	manager.mu.Unlock()
	return result, nil
}

func (manager *runManager) status(runID string) map[string]any {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	record := manager.runs[runID]
	if record == nil {
		return map[string]any{"run_id": runID, "state": "unknown"}
	}
	return manager.summaryLocked(runID, record)
}

func (manager *runManager) result(runID string) map[string]any {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	record := manager.runs[runID]
	if record == nil {
		return map[string]any{"run_id": runID, "state": "unknown"}
	}
	result := manager.summaryLocked(runID, record)
	result["output"] = record.output
	result["output_truncated"] = record.outputTruncated
	return result
}

func (manager *runManager) list() []map[string]any {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	ids := make([]string, 0, len(manager.runs))
	for id := range manager.runs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		result = append(result, manager.summaryLocked(id, manager.runs[id]))
	}
	return result
}

func (manager *runManager) events(runID string, after uint64, limit int) map[string]any {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	record := manager.runs[runID]
	if record == nil {
		return map[string]any{
			"run_id": runID, "state": "unknown", "events": []any{},
			"next_seq": uint64(0), "truncated": false,
		}
	}
	firstSeq := record.nextSeq
	if len(record.events) > 0 {
		firstSeq = record.events[0].Seq
	}
	if limit <= 0 || limit > maxEventPage {
		limit = maxEventPage
	}
	page := make([]map[string]any, 0, limit)
	for _, event := range record.events {
		if event.Seq <= after {
			continue
		}
		page = append(page, map[string]any{"seq": event.Seq, "type": event.Type, "text": event.Text})
		if len(page) == limit {
			break
		}
	}
	return map[string]any{
		"run_id": runID, "events": page, "next_seq": record.nextSeq,
		"truncated": after+1 < firstSeq,
	}
}

func (manager *runManager) cancel(runID string) map[string]any {
	var child *exec.Cmd
	manager.mu.Lock()
	record := manager.runs[runID]
	if record == nil {
		manager.mu.Unlock()
		return map[string]any{"run_id": runID, "state": "unknown"}
	}
	if record.state.terminal() {
		result := manager.summaryLocked(runID, record)
		manager.mu.Unlock()
		return result
	}
	record.cancelRequested = true
	if record.state == runQueued {
		filtered := manager.queue[:0]
		for _, job := range manager.queue {
			if job.id != runID {
				filtered = append(filtered, job)
			}
		}
		manager.queue = filtered
		record.state = runCancelled
		finished := nowMillis()
		record.finishedAt = &finished
		pushRunEvent(record, "run_cancelled", "")
		manager.pruneTerminalLocked()
		manager.dispatchLocked()
	} else {
		child = record.child
	}
	result := manager.summaryLocked(runID, record)
	manager.mu.Unlock()
	if child != nil {
		stopExecProcessTree(child)
	}
	return result
}

func (manager *runManager) shutdown() {
	var children []*exec.Cmd
	manager.mu.Lock()
	if manager.shuttingDown {
		manager.mu.Unlock()
		return
	}
	manager.shuttingDown = true
	manager.queue = nil
	now := nowMillis()
	for _, record := range manager.runs {
		if record.state.terminal() {
			continue
		}
		record.cancelRequested = true
		if record.state == runQueued {
			record.state = runCancelled
			record.finishedAt = &now
			pushRunEvent(record, "run_cancelled", "")
		} else if record.child != nil {
			children = append(children, record.child)
		}
	}
	manager.mu.Unlock()
	for _, child := range children {
		stopExecProcessTree(child)
	}
	done := make(chan struct{})
	go func() {
		manager.workers.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}

func (manager *runManager) dispatchLocked() {
	if manager.shuttingDown {
		return
	}
	for manager.active < maxActiveRuns && len(manager.queue) > 0 {
		job := manager.queue[0]
		manager.queue = manager.queue[1:]
		record := manager.runs[job.id]
		if record == nil || record.state != runQueued {
			continue
		}
		manager.active++
		record.state = runStarting
		started := nowMillis()
		record.startedAt = &started
		pushRunEvent(record, "run_started", "")
		manager.workers.Add(1)
		go manager.execute(job)
	}
}

func (manager *runManager) execute(job queuedRun) {
	defer manager.workers.Done()
	command := exec.Command(manager.executable, job.args...)
	command.Dir = manager.workspace
	configureExecProcess(command)
	command.Env = childRunEnvironment(job.apiEnv, manager.instanceID, manager.workspaceName)
	stdin, err := command.StdinPipe()
	if err != nil {
		manager.finishStartFailed(job.id, "capture stdin")
		return
	}
	command.Stdout = runOutputWriter{manager: manager, runID: job.id, eventType: "stdout"}
	command.Stderr = runOutputWriter{manager: manager, runID: job.id, eventType: "stderr"}
	if err := command.Start(); err != nil {
		manager.finishStartFailed(job.id, "spawn")
		return
	}

	manager.mu.Lock()
	record := manager.runs[job.id]
	cancelled := record == nil || record.cancelRequested
	if record != nil {
		record.child = command
		if !cancelled && record.state == runStarting {
			record.state = runRunning
		}
	}
	manager.mu.Unlock()
	if cancelled {
		stopExecProcessTree(command)
	}

	_, writeErr := io.WriteString(stdin, job.task)
	if writeErr == nil {
		writeErr = stdin.Close()
	} else {
		_ = stdin.Close()
	}
	if writeErr != nil {
		manager.mu.Lock()
		if current := manager.runs[job.id]; current != nil {
			current.cancelRequested = true
		}
		manager.mu.Unlock()
		stopExecProcessTree(command)
	}
	waitErr := command.Wait()

	manager.mu.Lock()
	record = manager.runs[job.id]
	if record != nil {
		record.child = nil
		finished := nowMillis()
		record.finishedAt = &finished
		if record.cancelRequested {
			record.state = runCancelled
			pushRunEvent(record, "run_cancelled", "")
		} else if waitErr == nil {
			code := 0
			record.exitStatus = &code
			record.state = runSucceeded
			pushRunEvent(record, "run_succeeded", "")
		} else {
			code := processExitCode(waitErr)
			record.exitStatus = &code
			record.state = runFailed
			pushRunEvent(record, "run_failed", "")
		}
	}
	if manager.active > 0 {
		manager.active--
	}
	manager.pruneTerminalLocked()
	manager.dispatchLocked()
	manager.mu.Unlock()
}

type runOutputWriter struct {
	manager   *runManager
	runID     string
	eventType string
}

func (writer runOutputWriter) Write(content []byte) (int, error) {
	for start := 0; start < len(content); start += 4096 {
		end := min(start+4096, len(content))
		writer.manager.appendRunOutput(writer.runID, writer.eventType, content[start:end])
	}
	return len(content), nil
}

func (manager *runManager) appendRunOutput(runID, eventType string, content []byte) {
	text := redact.Secrets(string(content))
	text = boundUTF8(text, maxRunEventTextBytes)
	if text == "" {
		return
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	record := manager.runs[runID]
	if record == nil {
		return
	}
	if len(record.output) < runOutputMaxBytes {
		remaining := runOutputMaxBytes - len(record.output)
		appendText := boundUTF8(text, remaining)
		record.output += appendText
		if len(appendText) < len(text) {
			record.outputTruncated = true
		}
	} else {
		record.outputTruncated = true
	}
	pushRunEvent(record, eventType, text)
	if record.outputTruncated && !hasRunEvent(record, "output_truncated") {
		pushRunEvent(record, "output_truncated", "output limit reached")
	}
}

func (manager *runManager) finishStartFailed(runID, message string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	record := manager.runs[runID]
	if record != nil && !record.state.terminal() {
		finished := nowMillis()
		record.finishedAt = &finished
		if record.cancelRequested {
			record.state = runCancelled
			pushRunEvent(record, "run_cancelled", "")
		} else {
			record.state = runStartFailed
			manager.appendOutputLocked(record, "stderr", message)
			pushRunEvent(record, "run_start_failed", "Super child could not start")
		}
	}
	if manager.active > 0 {
		manager.active--
	}
	manager.pruneTerminalLocked()
	manager.dispatchLocked()
}

func (manager *runManager) appendOutputLocked(record *runRecord, eventType, text string) {
	text = boundUTF8(redact.Secrets(text), maxRunEventTextBytes)
	if len(record.output) < runOutputMaxBytes {
		remaining := runOutputMaxBytes - len(record.output)
		record.output += boundUTF8(text, remaining)
	}
	pushRunEvent(record, eventType, text)
}

func (manager *runManager) summaryLocked(runID string, record *runRecord) map[string]any {
	result := map[string]any{
		"run_id": runID, "state": string(record.state),
		"created_at": record.createdAt, "started_at": nullableUint(record.startedAt),
		"finished_at": nullableUint(record.finishedAt), "exit_status": nullableInt(record.exitStatus),
		"provider": nullableString(record.provider), "model": nullableString(record.model),
		"reasoning_effort":       nullableString(record.reasoningEffort),
		"cancellation_requested": record.cancelRequested,
	}
	return result
}

func (manager *runManager) pruneTerminalLocked() {
	for terminalRunCount(manager.runs) > maxRetainedTerminalRuns {
		oldestID := ""
		var oldestFinished, oldestCreated uint64
		for id, record := range manager.runs {
			if !record.state.terminal() {
				continue
			}
			finished := record.createdAt
			if record.finishedAt != nil {
				finished = *record.finishedAt
			}
			if oldestID == "" || finished < oldestFinished ||
				(finished == oldestFinished && record.createdAt < oldestCreated) {
				oldestID, oldestFinished, oldestCreated = id, finished, record.createdAt
			}
		}
		if oldestID == "" {
			return
		}
		delete(manager.runs, oldestID)
	}
}

func terminalRunCount(runs map[string]*runRecord) int {
	count := 0
	for _, record := range runs {
		if record.state.terminal() {
			count++
		}
	}
	return count
}

func pushRunEvent(record *runRecord, eventType, text string) {
	record.events = append(record.events, runEvent{
		Seq: record.nextSeq, Type: eventType, Text: boundUTF8(redact.Secrets(text), maxRunEventTextBytes),
	})
	record.nextSeq++
	if len(record.events) > maxRunEvents {
		record.events = record.events[len(record.events)-maxRunEvents:]
	}
}

func hasRunEvent(record *runRecord, eventType string) bool {
	for _, event := range record.events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

func newRunID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate expose run id: %w", err)
	}
	return "gsr_" + base64.RawURLEncoding.EncodeToString(bytes), nil
}

func validRunID(value string) bool {
	if len(value) < 4 || len(value) > 128 ||
		(!strings.HasPrefix(value, "gsr_") && !strings.HasPrefix(value, "spr_")) {
		return false
	}
	for _, current := range value {
		if (current >= '0' && current <= '9') || (current >= 'A' && current <= 'Z') ||
			(current >= 'a' && current <= 'z') || current == '_' || current == '-' {
			continue
		}
		return false
	}
	return true
}

type runMetadata struct {
	provider  string
	model     string
	reasoning string
}

func (manager *runManager) buildChildArgs(overrides map[string]any) ([]string, runMetadata, map[string]string, error) {
	base, apiKey := sanitizeExposeBaseArgs(manager.baseArgs)
	baseProvider, baseURL := superProviderFromArgs(base)
	provider := baseProvider
	if provider == "" {
		provider = "openai"
	}
	providerOverride, err := optionalRunString(overrides, "provider", 256)
	if err != nil {
		return nil, runMetadata{}, nil, err
	}
	if providerOverride != "" {
		normalized, err := normalizeExposeProvider(providerOverride)
		if err != nil {
			return nil, runMetadata{}, nil, err
		}
		if normalized != provider {
			apiKey = ""
		}
		base = stripExposeProviderArgs(base)
		switch normalized {
		case "openai":
			provider = "openai"
		case "local":
			if baseURL == "" {
				return nil, runMetadata{}, nil, errors.New("local provider requires the expose local URL")
			}
			base = append(base, "--url", baseURL)
			provider = "local"
		default:
			base = append(base, "--provider", normalized)
			provider = normalized
		}
	}

	if model, err := optionalRunString(overrides, "model", 256); err != nil {
		return nil, runMetadata{}, nil, err
	} else if model != "" {
		base = stripNamedOption(base, "--model", "--local-model")
		base = append(base, "--model", model)
	}
	if profile, err := optionalRunString(overrides, "profile", 128); err != nil {
		return nil, runMetadata{}, nil, err
	} else if profile != "" {
		if !validExposeProfileName(profile) {
			return nil, runMetadata{}, nil, errors.New("profile is invalid")
		}
		base = stripNamedOption(base, "--profile", "-p")
		base = append(base, "--profile", profile)
	}
	if effort, err := optionalRunString(overrides, "reasoning_effort", 256); err != nil {
		return nil, runMetadata{}, nil, err
	} else if effort != "" {
		effort = strings.ToLower(effort)
		if !validExposeReasoningEffort(effort) {
			return nil, runMetadata{}, nil, errors.New("reasoning_effort is unsupported")
		}
		base = stripConfigOverride(base, "model_reasoning_effort")
		base = append(base, "-c", "model_reasoning_effort="+strconv.Quote(effort))
	}
	if value, exists := overrides["sub_agents"]; exists && value != nil {
		enabled, ok := value.(bool)
		if !ok {
			return nil, runMetadata{}, nil, errors.New("sub_agents must be a boolean")
		}
		base = stripSubAgentArgs(base, !enabled)
		if enabled {
			base = append(base, "--sub-agent")
		} else {
			base = append(base, "--no-sub-agent")
		}
	}

	metadata := metadataFromExposeArgs(base)
	if metadata.provider == "" {
		metadata.provider = provider
	}
	child := []string{"s", "--full-access"}
	child = append(child, base...)
	child = append(child, "exec", "-")
	apiEnv := map[string]string{}
	if apiKey != "" {
		switch metadata.provider {
		case "anthropic":
			apiEnv["ANTHROPIC_API_KEY"] = apiKey
		case "copilot":
			apiEnv["GITHUB_COPILOT_API_KEY"] = apiKey
		case "deepseek":
			apiEnv["DEEPSEEK_API_KEY"] = apiKey
		case "gemini":
			apiEnv["GEMINI_API_KEY"] = apiKey
		}
	}
	return child, metadata, apiEnv, nil
}

func sanitizeExposeBaseArgs(arguments []string) ([]string, string) {
	result := make([]string, 0, len(arguments))
	apiKey := ""
	for index := 0; index < len(arguments); {
		argument := arguments[index]
		if argument == "--dry-run" || argument == "--full-access" {
			index++
			continue
		}
		if argument == "--api-key" && index+1 < len(arguments) {
			apiKey = arguments[index+1]
			index += 2
			continue
		}
		if strings.HasPrefix(argument, "--api-key=") {
			apiKey = strings.TrimPrefix(argument, "--api-key=")
			index++
			continue
		}
		result = append(result, argument)
		index++
	}
	return result, apiKey
}

func superProviderFromArgs(arguments []string) (string, string) {
	provider, localURL := "openai", ""
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		switch {
		case index == 0 && (argument == "gemini" || argument == "deepseek"):
			provider = argument
		case argument == "--provider" && index+1 < len(arguments):
			provider, _ = normalizeExposeProvider(arguments[index+1])
			index++
		case strings.HasPrefix(argument, "--provider="):
			provider, _ = normalizeExposeProvider(strings.TrimPrefix(argument, "--provider="))
		case argument == "--url" && index+1 < len(arguments):
			localURL = arguments[index+1]
			provider = "local"
			index++
		case strings.HasPrefix(argument, "--url="):
			localURL = strings.TrimPrefix(argument, "--url=")
			provider = "local"
		}
	}
	return provider, localURL
}

func normalizeExposeProvider(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "openai":
		return "openai", nil
	case "local":
		return "local", nil
	case "anthropic", "claude":
		return "anthropic", nil
	case "copilot", "github-copilot", "github_copilot":
		return "copilot", nil
	case "deepseek":
		return "deepseek", nil
	case "gemini", "google":
		return "gemini", nil
	case "kiro":
		return "kiro", nil
	default:
		return "", errors.New("provider is unsupported")
	}
}

func stripExposeProviderArgs(arguments []string) []string {
	result := make([]string, 0, len(arguments))
	for index := 0; index < len(arguments); {
		argument := arguments[index]
		if index == 0 && (argument == "gemini" || argument == "deepseek") {
			index++
			continue
		}
		if argument == "--provider" || argument == "--url" || argument == "--api-key" {
			index += 2
			continue
		}
		if strings.HasPrefix(argument, "--provider=") || strings.HasPrefix(argument, "--url=") ||
			strings.HasPrefix(argument, "--api-key=") {
			index++
			continue
		}
		result = append(result, argument)
		index++
	}
	return result
}

func stripNamedOption(arguments []string, names ...string) []string {
	result := make([]string, 0, len(arguments))
	for index := 0; index < len(arguments); {
		matched := false
		for _, name := range names {
			if arguments[index] == name {
				index += 2
				matched = true
				break
			}
			if strings.HasPrefix(arguments[index], name+"=") {
				index++
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		result = append(result, arguments[index])
		index++
	}
	return result
}

func stripSubAgentArgs(arguments []string, clearDetails bool) []string {
	result := make([]string, 0, len(arguments))
	details := []string{
		"--sub-agent-provider", "--sub-agent-model", "--sub-agent-model-reasoning-effort",
		"--sub-agent-url", "--sub-agent-max-concurrency",
	}
	for index := 0; index < len(arguments); {
		argument := arguments[index]
		if argument == "--sub-agent" || argument == "--no-sub-agent" {
			index++
			continue
		}
		if clearDetails {
			matched := false
			for _, name := range details {
				if argument == name {
					index += 2
					matched = true
					break
				}
				if strings.HasPrefix(argument, name+"=") {
					index++
					matched = true
					break
				}
			}
			if matched {
				continue
			}
		}
		result = append(result, argument)
		index++
	}
	return result
}

func stripConfigOverride(arguments []string, key string) []string {
	result := make([]string, 0, len(arguments))
	for index := 0; index < len(arguments); {
		argument := arguments[index]
		if (argument == "-c" || argument == "--config") && index+1 < len(arguments) {
			if configAssignmentKey(arguments[index+1]) == key {
				index += 2
				continue
			}
			result = append(result, argument, arguments[index+1])
			index += 2
			continue
		}
		if strings.HasPrefix(argument, "--config=") {
			if configAssignmentKey(strings.TrimPrefix(argument, "--config=")) == key {
				index++
				continue
			}
		}
		if strings.HasPrefix(argument, "-c") && argument != "-C" {
			value := strings.TrimPrefix(strings.TrimPrefix(argument, "-c"), "=")
			if configAssignmentKey(value) == key {
				index++
				continue
			}
		}
		result = append(result, argument)
		index++
	}
	return result
}

func configAssignmentKey(value string) string {
	key, _, _ := strings.Cut(strings.TrimSpace(value), "=")
	return strings.Trim(strings.TrimSpace(key), "\"'")
}

func metadataFromExposeArgs(arguments []string) runMetadata {
	provider, _ := superProviderFromArgs(arguments)
	metadata := runMetadata{provider: provider}
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if (argument == "--model" || argument == "--local-model") && index+1 < len(arguments) {
			metadata.model = arguments[index+1]
			index++
			continue
		}
		if strings.HasPrefix(argument, "--model=") {
			metadata.model = strings.TrimPrefix(argument, "--model=")
		}
		if strings.HasPrefix(argument, "--local-model=") {
			metadata.model = strings.TrimPrefix(argument, "--local-model=")
		}
		value := ""
		if (argument == "-c" || argument == "--config") && index+1 < len(arguments) {
			value = arguments[index+1]
			index++
		} else if strings.HasPrefix(argument, "--config=") {
			value = strings.TrimPrefix(argument, "--config=")
		} else if strings.HasPrefix(argument, "-c") && argument != "-C" {
			value = strings.TrimPrefix(strings.TrimPrefix(argument, "-c"), "=")
		}
		if configAssignmentKey(value) == "model_reasoning_effort" {
			_, raw, _ := strings.Cut(value, "=")
			metadata.reasoning = strings.Trim(strings.TrimSpace(raw), "\"'")
		}
	}
	return metadata
}

func optionalRunString(arguments map[string]any, name string, max int) (string, error) {
	value, exists := arguments[name]
	if !exists || value == nil {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", name)
	}
	if text == "" || len(text) > max {
		return "", fmt.Errorf("%s is empty or too large", name)
	}
	for _, current := range text {
		if current <= 31 || (current >= 127 && current <= 159) {
			return "", fmt.Errorf("%s is empty or too large", name)
		}
	}
	return text, nil
}

func requiredBoundedString(arguments map[string]any, name string, max int) (string, error) {
	text, err := optionalRunString(arguments, name, max)
	if err != nil {
		return "", err
	}
	if text == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return text, nil
}

func validExposeReasoningEffort(value string) bool {
	switch value {
	case "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra":
		return true
	default:
		return false
	}
}

func validExposeProfileName(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, current := range value {
		if (current >= 'A' && current <= 'Z') || (current >= 'a' && current <= 'z') ||
			(current >= '0' && current <= '9') || current == '-' || current == '_' || current == '.' {
			continue
		}
		return false
	}
	return true
}

func childRunEnvironment(extra map[string]string, instanceID, workspaceName string) []string {
	environment := inheritedEnvironment()
	delete(environment, "CONTROL_PLANE_API_KEY")
	for key, value := range extra {
		environment[key] = value
	}
	environment["GODEX_EXPOSE_INSTANCE_ID"] = instanceID
	environment["GODEX_EXPOSE_WORKSPACE_NAME"] = workspaceName
	return flattenEnvironment(environment)
}

func processExitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if code := exitErr.ExitCode(); code >= 0 {
			return code
		}
		_, code := execExitSignal(exitErr)
		return code
	}
	return 1
}

func boundUTF8(value string, maximum int) string {
	if maximum <= 0 {
		return ""
	}
	if len(value) <= maximum {
		return value
	}
	end := maximum
	for end > 0 && !utf8Boundary(value, end) {
		end--
	}
	return value[:end]
}

func utf8Boundary(value string, index int) bool {
	return index == 0 || index == len(value) || (value[index]&0xc0) != 0x80
}

func nullableUint(value *uint64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nowMillis() uint64 {
	return uint64(time.Now().UnixMilli())
}
