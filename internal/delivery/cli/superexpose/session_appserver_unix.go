//go:build linux

package superexpose

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	appServerMaxMessages    = 64
	appServerMaxMessageSize = 512 * 1024
	appServerRequestTimeout = 3 * time.Second
	preemptDrainAttempts    = 4
	preemptQueueLimit       = 100
)

type appServerOutcome uint8

const (
	appServerRejected appServerOutcome = iota
	appServerAccepted
	appServerAmbiguous
)

type appServerSocket struct {
	conn   net.Conn
	reader *bufio.Reader
}

type appServerActivity struct {
	active       bool
	activeTurnID string
}

func connectAppServerSocket(path string) (*appServerSocket, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("app-server socket path must be absolute")
	}
	conn, err := net.DialTimeout("unix", path, appServerRequestTimeout)
	if err != nil {
		return nil, err
	}
	socket := &appServerSocket{conn: conn, reader: bufio.NewReaderSize(conn, 16*1024)}
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		_ = conn.Close()
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)
	request := "GET /rpc HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if err := conn.SetWriteDeadline(time.Now().Add(appServerRequestTimeout)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if _, err := io.WriteString(conn, request); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := conn.SetReadDeadline(time.Now().Add(appServerRequestTimeout)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	response, err := http.ReadResponse(socket.reader, &http.Request{Method: http.MethodGet})
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if response.StatusCode != http.StatusSwitchingProtocols ||
		!strings.EqualFold(strings.TrimSpace(response.Header.Get("Upgrade")), "websocket") ||
		!headerContainsToken(response.Header.Get("Connection"), "upgrade") ||
		response.Header.Get("Sec-WebSocket-Accept") != websocketAccept(key) {
		_ = conn.Close()
		return nil, errors.New("app-server websocket handshake rejected")
	}
	return socket, nil
}

func headerContainsToken(value, token string) bool {
	for _, item := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(item), token) {
			return true
		}
	}
	return false
}

func websocketAccept(key string) string {
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func (socket *appServerSocket) close() {
	if socket != nil && socket.conn != nil {
		_ = socket.conn.Close()
	}
}

func (socket *appServerSocket) sendText(text string) error {
	return socket.writeFrame(0x1, []byte(text))
}

func (socket *appServerSocket) writeFrame(opcode byte, payload []byte) error {
	if len(payload) > appServerMaxMessageSize {
		return errors.New("app-server websocket payload is too large")
	}
	if err := socket.conn.SetWriteDeadline(time.Now().Add(appServerRequestTimeout)); err != nil {
		return err
	}
	mask := [4]byte{}
	if _, err := rand.Read(mask[:]); err != nil {
		return err
	}
	header := []byte{0x80 | opcode}
	length := len(payload)
	switch {
	case length < 126:
		header = append(header, 0x80|byte(length))
	case length <= 0xffff:
		header = append(header, 0x80|126, 0, 0)
		binary.BigEndian.PutUint16(header[len(header)-2:], uint16(length))
	default:
		header = append(header, 0x80|127, 0, 0, 0, 0, 0, 0, 0, 0)
		binary.BigEndian.PutUint64(header[len(header)-8:], uint64(length))
	}
	header = append(header, mask[:]...)
	masked := make([]byte, len(payload))
	for index := range payload {
		masked[index] = payload[index] ^ mask[index%4]
	}
	if _, err := socket.conn.Write(header); err != nil {
		return err
	}
	_, err := socket.conn.Write(masked)
	return err
}

func (socket *appServerSocket) readText() (string, error) {
	var message []byte
	started := false
	for {
		if err := socket.conn.SetReadDeadline(time.Now().Add(appServerRequestTimeout)); err != nil {
			return "", err
		}
		first, err := socket.reader.ReadByte()
		if err != nil {
			return "", err
		}
		second, err := socket.reader.ReadByte()
		if err != nil {
			return "", err
		}
		fin := first&0x80 != 0
		opcode := first & 0x0f
		masked := second&0x80 != 0
		length := uint64(second & 0x7f)
		switch length {
		case 126:
			var bytes [2]byte
			if _, err := io.ReadFull(socket.reader, bytes[:]); err != nil {
				return "", err
			}
			length = uint64(binary.BigEndian.Uint16(bytes[:]))
		case 127:
			var bytes [8]byte
			if _, err := io.ReadFull(socket.reader, bytes[:]); err != nil {
				return "", err
			}
			length = binary.BigEndian.Uint64(bytes[:])
		}
		if length > appServerMaxMessageSize || uint64(len(message))+length > appServerMaxMessageSize {
			return "", errors.New("app-server websocket message is too large")
		}
		var mask [4]byte
		if masked {
			if _, err := io.ReadFull(socket.reader, mask[:]); err != nil {
				return "", err
			}
		}
		payload := make([]byte, int(length))
		if _, err := io.ReadFull(socket.reader, payload); err != nil {
			return "", err
		}
		if masked {
			for index := range payload {
				payload[index] ^= mask[index%4]
			}
		}
		switch opcode {
		case 0x8:
			return "", errors.New("app-server websocket closed")
		case 0x9:
			if err := socket.writeFrame(0xA, payload); err != nil {
				return "", err
			}
			continue
		case 0xA:
			continue
		case 0x2:
			continue
		case 0x1:
			if started {
				return "", errors.New("app-server websocket started a second text message")
			}
			started = true
			message = append(message, payload...)
		case 0x0:
			if !started {
				return "", errors.New("app-server websocket continuation without text frame")
			}
			message = append(message, payload...)
		default:
			continue
		}
		if fin && started {
			return string(message), nil
		}
	}
}

func appServerRequest(socket *appServerSocket, requestID uint64, method string, params map[string]any) (appServerOutcome, map[string]any) {
	content, err := json.Marshal(map[string]any{
		"id": requestID, "method": method, "params": params,
	})
	if err != nil || socket.sendText(string(content)) != nil {
		return appServerAmbiguous, nil
	}
	for range appServerMaxMessages {
		text, err := socket.readText()
		if err != nil {
			return appServerAmbiguous, nil
		}
		var value map[string]any
		decoder := json.NewDecoder(strings.NewReader(text))
		decoder.UseNumber()
		if decoder.Decode(&value) != nil {
			return appServerAmbiguous, nil
		}
		if !jsonIDMatches(value["id"], requestID) {
			continue
		}
		if value["error"] != nil {
			return appServerRejected, nil
		}
		result, _ := value["result"].(map[string]any)
		if result == nil {
			result = map[string]any{}
		}
		return appServerAccepted, result
	}
	return appServerAmbiguous, nil
}

func jsonIDMatches(value any, expected uint64) bool {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Int64()
		return err == nil && parsed >= 0 && uint64(parsed) == expected
	case float64:
		return typed >= 0 && typed == float64(expected)
	default:
		return false
	}
}

func appServerNotification(socket *appServerSocket, method string) error {
	content, err := json.Marshal(map[string]any{"method": method})
	if err != nil {
		return err
	}
	return socket.sendText(string(content))
}

func openAppServerTarget(target resolvedSessionTarget, includeTurns bool) (*appServerSocket, *appServerActivity, error) {
	path := strings.TrimPrefix(target.remoteEndpoint, "unix://")
	if path == target.remoteEndpoint || !filepath.IsAbs(path) {
		return nil, nil, sessionNotQueueAddressable
	}
	socket, err := connectAppServerSocket(path)
	if err != nil {
		return nil, nil, sessionNotQueueAddressable
	}
	fail := func(err error) (*appServerSocket, *appServerActivity, error) {
		socket.close()
		return nil, nil, err
	}
	outcome, initialize := appServerRequest(socket, 1, "initialize", map[string]any{
		"clientInfo":   map[string]any{"name": "godex-session-bridge", "version": "0.435.6"},
		"capabilities": map[string]any{"experimentalApi": true},
	})
	if outcome != appServerAccepted {
		return fail(sessionNotQueueAddressable)
	}
	serverHome, _ := initialize["codexHome"].(string)
	if serverHome == "" || !sameCanonicalPath(serverHome, target.environment.codexHome) {
		return fail(sessionNotQueueAddressable)
	}
	if err := appServerNotification(socket, "initialized"); err != nil {
		return fail(sessionNotQueueAddressable)
	}
	outcome, result := appServerRequest(socket, 2, "thread/read", map[string]any{
		"threadId": target.threadID, "includeTurns": includeTurns,
	})
	if outcome != appServerAccepted {
		return fail(sessionNotQueueAddressable)
	}
	thread, _ := result["thread"].(map[string]any)
	if thread == nil {
		return fail(sessionNotQueueAddressable)
	}
	id, _ := thread["id"].(string)
	sessionID, _ := thread["sessionId"].(string)
	ephemeral, ephemeralOK := thread["ephemeral"].(bool)
	accepts, acceptsOK := thread["canAcceptDirectInput"].(bool)
	cwd, _ := thread["cwd"].(string)
	if id != target.threadID || sessionID != target.threadID ||
		!ephemeralOK || ephemeral || !acceptsOK || !accepts ||
		cwd == "" || !sameCanonicalPath(cwd, target.environment.pwd) {
		return fail(sessionNotQueueAddressable)
	}
	status, _ := thread["status"].(map[string]any)
	statusType, _ := status["type"].(string)
	activity := &appServerActivity{}
	switch statusType {
	case "active":
		activity.active = true
	case "idle":
	default:
		return fail(sessionNotQueueAddressable)
	}
	if includeTurns {
		turns, ok := thread["turns"].([]any)
		if !ok {
			return fail(sessionVerificationInconclusive)
		}
		for _, raw := range turns {
			turn, _ := raw.(map[string]any)
			if turn == nil {
				continue
			}
			turnStatus, _ := turn["status"].(map[string]any)
			turnType, _ := turnStatus["type"].(string)
			if turnType != "inProgress" {
				continue
			}
			turnID, _ := turn["id"].(string)
			if turnID == "" || activity.activeTurnID != "" {
				return fail(sessionVerificationInconclusive)
			}
			activity.activeTurnID = turnID
		}
	}
	return socket, activity, nil
}

func appServerThreadActivity(target resolvedSessionTarget, includeTurns bool) (*appServerActivity, bool, error) {
	socket, activity, err := openAppServerTarget(target, includeTurns)
	if err != nil {
		if errors.Is(err, sessionNotQueueAddressable) {
			return nil, false, nil
		}
		return nil, false, err
	}
	socket.close()
	return activity, true, nil
}

func appServerQueueAddOnce(target resolvedSessionTarget, message string) queueInvocation {
	socket, _, err := openAppServerTarget(target, false)
	if err != nil {
		return queueInvocation{outcome: queuePreflight}
	}
	defer socket.close()
	messageUUID, err := uuid.NewV7()
	if err != nil {
		return queueInvocation{outcome: queueAmbiguous}
	}
	messageID := messageUUID.String()
	outcome, result := appServerRequest(socket, 3, "thread/queue/add", map[string]any{
		"threadId":            target.threadID,
		"clientUserMessageId": messageID,
		"input":               []any{map[string]any{"type": "text", "text": message, "textElements": []any{}}},
	})
	switch outcome {
	case appServerRejected:
		return queueInvocation{outcome: queueRejected}
	case appServerAmbiguous:
		return queueInvocation{outcome: queueAmbiguous}
	}
	submission, _ := result["queuedSubmission"].(map[string]any)
	if submission == nil {
		return queueInvocation{outcome: queueAmbiguous}
	}
	submissionID, _ := submission["id"].(string)
	echoID, _ := submission["clientUserMessageId"].(string)
	input, _ := submission["input"].([]any)
	if submissionID == "" || echoID != messageID || len(input) != 1 {
		return queueInvocation{outcome: queueAmbiguous}
	}
	text, _ := input[0].(map[string]any)
	if text == nil || text["type"] != "text" || text["text"] != message {
		return queueInvocation{outcome: queueAmbiguous}
	}
	if elements, exists := text["textElements"]; exists {
		array, ok := elements.([]any)
		if !ok || len(array) != 0 {
			return queueInvocation{outcome: queueAmbiguous}
		}
	}
	return queueInvocationAccepted(messageID, submissionID, true)
}

func appServerPreempt(target resolvedSessionTarget) (queuePreemptResult, error) {
	socket, initial, err := openAppServerTarget(target, true)
	if err != nil {
		return queuePreemptResult{}, err
	}
	defer socket.close()
	if initial.active && initial.activeTurnID == "" {
		return queuePreemptResult{}, sessionVerificationInconclusive
	}
	requestID := uint64(3)
	cancelled, empty, _, err := drainAppServerQueue(socket, target, &requestID)
	if err != nil || !empty {
		if err != nil {
			return queuePreemptResult{}, err
		}
		return queuePreemptResult{}, sessionVerificationInconclusive
	}
	boundary, err := appServerThreadRead(socket, target, true, &requestID)
	if err != nil {
		return queuePreemptResult{}, err
	}
	if boundary.active && boundary.activeTurnID == "" {
		return queuePreemptResult{}, sessionVerificationInconclusive
	}
	turnToInterrupt := ""
	switch {
	case initial.activeTurnID != "" && boundary.activeTurnID == initial.activeTurnID:
		turnToInterrupt = boundary.activeTurnID
	case initial.activeTurnID != "" && boundary.activeTurnID == "":
	case initial.activeTurnID == "" && boundary.activeTurnID == "":
	default:
		return queuePreemptResult{}, sessionVerificationInconclusive
	}
	interrupted := false
	if turnToInterrupt != "" {
		outcome, _ := appServerRequest(socket, nextAppServerRequestID(&requestID), "turn/interrupt", map[string]any{
			"threadId": target.threadID, "turnId": turnToInterrupt,
		})
		switch outcome {
		case appServerAccepted:
			interrupted = true
		case appServerRejected:
		case appServerAmbiguous:
			return queuePreemptResult{}, sessionVerificationInconclusive
		}
	}
	final, err := appServerThreadRead(socket, target, true, &requestID)
	if err != nil || final.activeTurnID != "" {
		return queuePreemptResult{}, sessionVerificationInconclusive
	}
	remaining, err := appServerQueueList(socket, target, &requestID)
	if err != nil || len(remaining) != 0 {
		return queuePreemptResult{}, sessionVerificationInconclusive
	}
	return queuePreemptResult{
		currentTurnID:          initial.activeTurnID,
		currentTurnInterrupted: interrupted,
		cancelledSubmissionIDs: cancelled,
		remainingSubmissionIDs: remaining,
		queueEmptyAtBoundary:   true,
		sessionReady:           !final.active && final.activeTurnID == "" && len(remaining) == 0,
	}, nil
}

func appServerThreadRead(socket *appServerSocket, target resolvedSessionTarget, includeTurns bool, requestID *uint64) (*appServerActivity, error) {
	outcome, result := appServerRequest(socket, nextAppServerRequestID(requestID), "thread/read", map[string]any{
		"threadId": target.threadID, "includeTurns": includeTurns,
	})
	if outcome != appServerAccepted {
		return nil, sessionNotQueueAddressable
	}
	thread, _ := result["thread"].(map[string]any)
	if thread == nil || thread["id"] != target.threadID || thread["sessionId"] != target.threadID ||
		thread["ephemeral"] != false || thread["canAcceptDirectInput"] != true {
		return nil, sessionNotQueueAddressable
	}
	cwd, _ := thread["cwd"].(string)
	if !sameCanonicalPath(cwd, target.environment.pwd) {
		return nil, sessionNotQueueAddressable
	}
	status, _ := thread["status"].(map[string]any)
	statusType, _ := status["type"].(string)
	activity := &appServerActivity{}
	switch statusType {
	case "active":
		activity.active = true
	case "idle":
	default:
		return nil, sessionNotQueueAddressable
	}
	if includeTurns {
		turns, ok := thread["turns"].([]any)
		if !ok {
			return nil, sessionVerificationInconclusive
		}
		for _, raw := range turns {
			turn, _ := raw.(map[string]any)
			turnStatus, _ := turn["status"].(map[string]any)
			if turnStatus["type"] != "inProgress" {
				continue
			}
			id, _ := turn["id"].(string)
			if id == "" || activity.activeTurnID != "" {
				return nil, sessionVerificationInconclusive
			}
			activity.activeTurnID = id
		}
	}
	return activity, nil
}

func drainAppServerQueue(socket *appServerSocket, target resolvedSessionTarget, requestID *uint64) ([]string, bool, bool, error) {
	cancelled := []string{}
	hadPending := false
	for range preemptDrainAttempts {
		queued, err := appServerQueueList(socket, target, requestID)
		if err != nil {
			return nil, false, hadPending, err
		}
		if len(queued) == 0 {
			return cancelled, true, hadPending, nil
		}
		hadPending = true
		for _, submissionID := range queued {
			deleted, err := appServerQueueDelete(socket, target, requestID, submissionID)
			if err != nil {
				return nil, false, hadPending, err
			}
			if deleted {
				cancelled = append(cancelled, submissionID)
			}
		}
	}
	return cancelled, false, hadPending, nil
}

func appServerQueueList(socket *appServerSocket, target resolvedSessionTarget, requestID *uint64) ([]string, error) {
	outcome, result := appServerRequest(socket, nextAppServerRequestID(requestID), "thread/queue/list", map[string]any{
		"threadId": target.threadID, "limit": preemptQueueLimit,
	})
	if outcome == appServerRejected {
		return nil, sessionQueueFailed
	}
	if outcome != appServerAccepted {
		return nil, sessionVerificationInconclusive
	}
	if cursor, exists := result["nextCursor"]; exists && cursor != nil {
		return nil, sessionVerificationInconclusive
	}
	data, ok := result["data"].([]any)
	if !ok {
		return nil, sessionVerificationInconclusive
	}
	seen := map[string]bool{}
	ids := make([]string, 0, len(data))
	for _, raw := range data {
		item, _ := raw.(map[string]any)
		id, _ := item["id"].(string)
		if id == "" || len(id) > 128 || strings.IndexFunc(id, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 || seen[id] {
			return nil, sessionVerificationInconclusive
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, nil
}

func appServerQueueDelete(socket *appServerSocket, target resolvedSessionTarget, requestID *uint64, submissionID string) (bool, error) {
	outcome, result := appServerRequest(socket, nextAppServerRequestID(requestID), "thread/queue/delete", map[string]any{
		"threadId": target.threadID, "queuedSubmissionId": submissionID,
	})
	if outcome == appServerRejected {
		return false, sessionQueueFailed
	}
	if outcome != appServerAccepted {
		return false, sessionVerificationInconclusive
	}
	deleted, ok := result["deleted"].(bool)
	if !ok {
		return false, sessionVerificationInconclusive
	}
	return deleted, nil
}

func nextAppServerRequestID(value *uint64) uint64 {
	current := *value
	if *value != ^uint64(0) {
		(*value)++
	}
	return current
}
