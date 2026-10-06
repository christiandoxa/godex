package kiro

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const acpBufferedOutputMaxBytes = 8 << 20

type acpEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *acpError       `json:"error,omitempty"`
}

type acpError struct {
	Code    int64  `json:"code"`
	Message string `json:"message"`
}

type acpSession struct {
	SessionID string         `json:"sessionId"`
	Models    *acpModelState `json:"models,omitempty"`
}

type acpModelState struct {
	CurrentModelID  string         `json:"currentModelId"`
	AvailableModels []acpModelInfo `json:"availableModels"`
}

type acpModelInfo struct {
	ModelID string `json:"modelId"`
	Name    string `json:"name"`
}

type acpTurn struct {
	Session       acpSession
	Prompt        acpEnvelope
	Notifications []acpEnvelope
}

type acpTurnRunner func(context.Context, string, string, string, string) (acpTurn, error)

type acpChild struct {
	command          *exec.Cmd
	writer           *bufio.Writer
	scanner          *bufio.Scanner
	ownsProcessGroup bool
	stopOnce         sync.Once
	done             chan struct{}
}

type acpCollector struct {
	prompt         string
	initialized    bool
	promptSent     bool
	session        acpSession
	promptReply    *acpEnvelope
	notifications  []acpEnvelope
	onNotification func(acpEnvelope) error
}

func (source *Source) executeACPTurn(ctx context.Context, home, model, effort, prompt string) (acpTurn, error) {
	return source.executeACPTurnObserved(ctx, home, model, effort, prompt, nil)
}

func (source *Source) executeACPTurnObserved(
	ctx context.Context,
	home, model, effort, prompt string,
	onNotification func(acpEnvelope) error,
) (acpTurn, error) {
	if source != nil && source.acp != nil {
		return source.executeInjectedACPTurn(ctx, home, model, effort, prompt, onNotification)
	}
	return source.runACPTurnObserved(ctx, home, model, effort, prompt, onNotification)
}

func (source *Source) executeInjectedACPTurn(
	ctx context.Context,
	home, model, effort, prompt string,
	onNotification func(acpEnvelope) error,
) (acpTurn, error) {
	turn, err := source.acp(ctx, home, model, effort, prompt)
	if err != nil {
		return acpTurn{}, err
	}
	if err := notifyACPUpdates(turn.Notifications, onNotification); err != nil {
		return acpTurn{}, err
	}
	return turn, nil
}

func notifyACPUpdates(notifications []acpEnvelope, observe func(acpEnvelope) error) error {
	if observe == nil {
		return nil
	}
	for _, notification := range notifications {
		if notification.Method != "session/update" {
			continue
		}
		if err := observe(notification); err != nil {
			return err
		}
	}
	return nil
}

func (source *Source) runACPTurn(ctx context.Context, home, model, effort, prompt string) (acpTurn, error) {
	return source.runACPTurnObserved(ctx, home, model, effort, prompt, nil)
}

func (source *Source) runACPTurnObserved(
	ctx context.Context,
	home, model, effort, prompt string,
	onNotification func(acpEnvelope) error,
) (acpTurn, error) {
	child, err := source.startACPChild(ctx, home, model, effort)
	if err != nil {
		return acpTurn{}, err
	}
	defer child.stop()
	if err := bootstrapACP(child.writer, home); err != nil {
		return acpTurn{}, err
	}
	collector := acpCollector{prompt: prompt, onNotification: onNotification}
	if err := collectACPOutput(ctx, child.scanner, child.writer, &collector); err != nil {
		return acpTurn{}, err
	}
	return collector.turn(ctx)
}

func (source *Source) startACPChild(ctx context.Context, home, model, effort string) (*acpChild, error) {
	credential, err := source.prepareRuntimeCredential(ctx, home)
	if err != nil {
		return nil, err
	}
	arguments := []string{"acp", "--model", runtimeString(model, "auto")}
	if effort = strings.TrimSpace(effort); effort != "" {
		arguments = append(arguments, "--effort", effort)
	}
	command := exec.CommandContext(ctx, source.binary(), arguments...)
	command.Dir = filepath.Clean(home)
	ownsProcessGroup := strings.TrimSpace(source.getenv("GODEX_SUB_AGENT")) == "" && strings.TrimSpace(source.getenv("PRODEX_SUB_AGENT")) == ""
	configureACPProcess(command, ownsProcessGroup)
	command.Env = mergedEnvironment(source.databaseEnvironment(
		filepath.Join(credential.dataDir, kiroDatabaseFileName),
		quotaPointerValue(credential.secret.Region),
	))
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, errors.New("capture Kiro ACP stdin")
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, errors.New("capture Kiro ACP stdout")
	}
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("failed to start Kiro ACP agent: %w", err)
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64<<10), acpBufferedOutputMaxBytes)
	child := &acpChild{
		command: command, writer: bufio.NewWriter(stdin), scanner: scanner,
		ownsProcessGroup: ownsProcessGroup, done: make(chan struct{}),
	}
	child.watchContext(ctx)
	return child, nil
}

func (child *acpChild) watchContext(ctx context.Context) {
	if child == nil || child.done == nil {
		return
	}
	go func() {
		select {
		case <-ctx.Done():
			child.stop()
		case <-child.done:
		}
	}()
}

func (child *acpChild) stop() {
	if child == nil {
		return
	}
	child.stopOnce.Do(func() {
		if child.command != nil {
			terminateACPProcess(child.command, child.ownsProcessGroup)
			_ = child.command.Wait()
		}
		if child.done != nil {
			close(child.done)
		}
	})
}

func bootstrapACP(writer *bufio.Writer, home string) error {
	if err := writeACPRequest(writer, acpInitializeRequest(0)); err != nil {
		return err
	}
	if err := writeACPRequest(writer, acpSessionNewRequest(1, home)); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return errors.New("flush Kiro ACP bootstrap requests")
	}
	return nil
}

func collectACPOutput(ctx context.Context, scanner *bufio.Scanner, writer *bufio.Writer, collector *acpCollector) error {
	received := 0
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		received += len(line) + 1
		if received > acpBufferedOutputMaxBytes {
			return fmt.Errorf("Kiro ACP output exceeded safe size limit (%d)", acpBufferedOutputMaxBytes)
		}
		envelope, skip, err := decodeACPLine(line)
		if err != nil {
			return err
		}
		if skip {
			continue
		}
		done, err := collector.accept(writer, envelope)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read Kiro ACP stdout: %w", err)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

func decodeACPLine(line []byte) (acpEnvelope, bool, error) {
	if len(strings.TrimSpace(string(line))) == 0 {
		return acpEnvelope{}, true, nil
	}
	var envelope acpEnvelope
	if err := json.Unmarshal(line, &envelope); err != nil || envelope.JSONRPC != "2.0" {
		return acpEnvelope{}, false, errors.New("failed to parse Kiro ACP JSON-RPC line")
	}
	return envelope, false, nil
}

func (collector *acpCollector) accept(writer *bufio.Writer, envelope acpEnvelope) (bool, error) {
	if handled, err := handleACPServerRequest(writer, envelope); handled || err != nil {
		if handled {
			collector.notifications = append(collector.notifications, envelope)
		}
		return false, err
	}
	id, hasID := envelope.numericID()
	if !hasID {
		collector.notifications = append(collector.notifications, envelope)
		if collector.onNotification != nil && envelope.Method == "session/update" {
			if err := collector.onNotification(envelope); err != nil {
				return false, err
			}
		}
		return false, nil
	}
	if envelope.Error != nil && (id == 0 || id == 1) {
		return false, fmt.Errorf("Kiro ACP bootstrap failed: %s", envelope.Error.Message)
	}
	switch id {
	case 0:
		collector.initialized = envelope.Error == nil
		return false, nil
	case 1:
		return false, collector.acceptSession(writer, envelope)
	case 2:
		copy := envelope
		collector.promptReply = &copy
		return true, nil
	default:
		collector.notifications = append(collector.notifications, envelope)
		return false, nil
	}
}

func (collector *acpCollector) acceptSession(writer *bufio.Writer, envelope acpEnvelope) error {
	if envelope.Error != nil {
		return fmt.Errorf("Kiro ACP session/new failed: %s", envelope.Error.Message)
	}
	if err := json.Unmarshal(envelope.Result, &collector.session); err != nil || strings.TrimSpace(collector.session.SessionID) == "" {
		return errors.New("failed to parse Kiro ACP session/new result")
	}
	if collector.promptSent {
		return nil
	}
	if err := writeACPRequest(writer, acpSessionPromptRequest(2, collector.session.SessionID, collector.prompt)); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return errors.New("flush Kiro ACP session/prompt request")
	}
	collector.promptSent = true
	return nil
}

func (collector *acpCollector) turn(ctx context.Context) (acpTurn, error) {
	if ctx.Err() != nil {
		return acpTurn{}, ctx.Err()
	}
	if !collector.initialized || strings.TrimSpace(collector.session.SessionID) == "" || collector.promptReply == nil {
		return acpTurn{}, errors.New("Kiro ACP prompt turn ended before bootstrap/prompt completion")
	}
	return acpTurn{Session: collector.session, Prompt: *collector.promptReply, Notifications: collector.notifications}, nil
}

func (envelope acpEnvelope) numericID() (uint64, bool) {
	if len(envelope.ID) == 0 || string(envelope.ID) == "null" {
		return 0, false
	}
	var number uint64
	if json.Unmarshal(envelope.ID, &number) == nil {
		return number, true
	}
	var text string
	if json.Unmarshal(envelope.ID, &text) == nil {
		parsed, err := strconv.ParseUint(text, 10, 64)
		return parsed, err == nil
	}
	return 0, false
}

func writeACPRequest(writer *bufio.Writer, value any) error {
	content, err := json.Marshal(value)
	if err != nil {
		return errors.New("serialize Kiro ACP request")
	}
	content = append(content, '\n')
	if _, err := writer.Write(content); err != nil {
		return errors.New("write Kiro ACP request")
	}
	return nil
}

func acpInitializeRequest(id uint64) map[string]any {
	return map[string]any{
		kiroFieldJSONRPC: "2.0", "id": id, kiroFieldMethod: "initialize",
		kiroFieldParams: map[string]any{
			"protocolVersion": 1,
			"clientCapabilities": map[string]any{
				"fs":       map[string]any{"readTextFile": false, "writeTextFile": false},
				"terminal": false, "auth": map[string]any{"terminal": false},
			},
			"clientInfo": map[string]any{"name": "godex", "title": "Godex", "version": "0.435.1"},
		},
	}
}

func acpSessionNewRequest(id uint64, cwd string) map[string]any {
	return map[string]any{kiroFieldJSONRPC: "2.0", "id": id, kiroFieldMethod: "session/new", kiroFieldParams: map[string]any{"cwd": cwd, "mcpServers": []any{}}}
}

func acpSessionPromptRequest(id uint64, sessionID, prompt string) map[string]any {
	return map[string]any{
		kiroFieldJSONRPC: "2.0", "id": id, kiroFieldMethod: "session/prompt",
		kiroFieldParams: map[string]any{"sessionId": sessionID, "prompt": []any{map[string]any{"type": "text", "text": prompt}}},
	}
}

func handleACPServerRequest(writer *bufio.Writer, envelope acpEnvelope) (bool, error) {
	if len(envelope.ID) == 0 || strings.TrimSpace(envelope.Method) == "" {
		return false, nil
	}
	response := unsupportedACPServerRequest(envelope.ID)
	if envelope.Method == "session/request_permission" && kiroSubAgent() {
		response = permissionACPServerResponse(envelope.ID, envelope.Params)
	}
	if err := writeACPRequest(writer, response); err != nil {
		return true, err
	}
	return true, writer.Flush()
}
