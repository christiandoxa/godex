//go:build !windows

package codex

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

const (
	nativeRecoveryRequestTimeout = 3 * time.Second
	nativeRecoveryMaxMessages    = 64
	nativeRecoveryMaxMessageSize = 512 * 1024
)

type nativeRecoverySocket struct {
	conn   net.Conn
	reader *bufio.Reader
}

func connectNativeRecoverySocket(path string) (*nativeRecoverySocket, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("native recovery socket path must be absolute")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeSocket == 0 {
		return nil, errors.New("native recovery socket is not an owned Unix socket")
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && uint32(stat.Uid) != uint32(os.Getuid()) {
		return nil, errors.New("native recovery socket is owned by another user")
	}
	conn, err := net.DialTimeout("unix", path, nativeRecoveryRequestTimeout)
	if err != nil {
		return nil, err
	}
	socket := &nativeRecoverySocket{conn: conn, reader: bufio.NewReaderSize(conn, 16*1024)}
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		socket.close()
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)
	request := "GET /rpc HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	deadline := time.Now().Add(nativeRecoveryRequestTimeout)
	if err := conn.SetWriteDeadline(deadline); err != nil {
		socket.close()
		return nil, err
	}
	if _, err := io.WriteString(conn, request); err != nil {
		socket.close()
		return nil, err
	}
	if err := conn.SetReadDeadline(deadline); err != nil {
		socket.close()
		return nil, err
	}
	response, err := http.ReadResponse(socket.reader, &http.Request{Method: http.MethodGet})
	if err != nil {
		socket.close()
		return nil, err
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusSwitchingProtocols ||
		!strings.EqualFold(strings.TrimSpace(response.Header.Get("Upgrade")), "websocket") ||
		!headerHasToken(response.Header.Get("Connection"), "upgrade") ||
		response.Header.Get("Sec-WebSocket-Accept") != nativeWebSocketAccept(key) {
		socket.close()
		return nil, errors.New("native recovery websocket handshake rejected")
	}
	return socket, nil
}

func (socket *nativeRecoverySocket) close() {
	if socket != nil && socket.conn != nil {
		_ = socket.conn.Close()
	}
}

func (socket *nativeRecoverySocket) write(opcode byte, payload []byte, deadline time.Time) error {
	if socket == nil || socket.conn == nil || len(payload) > nativeRecoveryMaxMessageSize {
		return errors.New("native recovery websocket is unavailable")
	}
	if !deadline.After(time.Now()) {
		return os.ErrDeadlineExceeded
	}
	if err := socket.conn.SetWriteDeadline(deadline); err != nil {
		return err
	}
	return websocketframe.WriteFrame(socket.conn, opcode, payload, true)
}

func (socket *nativeRecoverySocket) readTextUntil(deadline time.Time) (string, error) {
	var message []byte
	started := false
	for {
		if !deadline.After(time.Now()) {
			return "", os.ErrDeadlineExceeded
		}
		if err := socket.conn.SetReadDeadline(deadline); err != nil {
			return "", err
		}
		frame, err := websocketframe.ReadHeader(socket.reader)
		if err != nil {
			return "", err
		}
		if frame.Header[0]&0x70 != 0 || frame.Masked() {
			return "", errors.New("invalid native recovery websocket frame")
		}
		payload, err := frame.ReadPayload(socket.reader, nativeRecoveryMaxMessageSize)
		if err != nil {
			return "", err
		}
		if frame.Opcode >= 0x8 && (!frame.Final || len(payload) > 125) {
			return "", errors.New("invalid native recovery websocket control frame")
		}
		switch frame.Opcode {
		case 0x8:
			return "", errors.New("native recovery websocket closed")
		case 0x9:
			if err := socket.write(0xA, payload, deadline); err != nil {
				return "", err
			}
		case 0xA, 0x2:
			continue
		case 0x1:
			if started {
				return "", errors.New("native recovery websocket started a second text message")
			}
			started = true
			message = append(message, payload...)
		case 0x0:
			if !started {
				return "", errors.New("native recovery websocket continuation without text")
			}
			message = append(message, payload...)
		default:
			continue
		}
		if len(message) > nativeRecoveryMaxMessageSize {
			return "", errors.New("native recovery websocket message is too large")
		}
		if started && frame.Final {
			return string(message), nil
		}
	}
}

func requestNativeRecovery(
	ctx context.Context, socket *nativeRecoverySocket, id uint64, method string, params map[string]any,
) (nativeRecoveryRequestOutcome, map[string]any) {
	if ctx.Err() != nil {
		return nativeRecoveryAmbiguous, nil
	}
	return requestNativeRecoveryUntil(ctx, socket, id, method, params, nativeRecoveryDeadline(ctx))
}

func requestNativeRecoveryUntil(
	ctx context.Context, socket *nativeRecoverySocket, id uint64, method string,
	params map[string]any, deadline time.Time,
) (nativeRecoveryRequestOutcome, map[string]any) {
	if ctx.Err() != nil {
		return nativeRecoveryAmbiguous, nil
	}
	content, err := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err != nil {
		return nativeRecoveryAmbiguous, nil
	}
	if err := socket.write(0x1, content, deadline); err != nil {
		return nativeRecoveryAmbiguous, nil
	}
	for range nativeRecoveryMaxMessages {
		if ctx.Err() != nil {
			return nativeRecoveryAmbiguous, nil
		}
		text, err := socket.readTextUntil(deadline)
		if err != nil {
			return nativeRecoveryAmbiguous, nil
		}
		var value map[string]any
		decoder := json.NewDecoder(strings.NewReader(text))
		decoder.UseNumber()
		if decoder.Decode(&value) != nil {
			return nativeRecoveryAmbiguous, nil
		}
		var trailing any
		if decoder.Decode(&trailing) != io.EOF {
			return nativeRecoveryAmbiguous, nil
		}
		if !nativeJSONIDMatches(value["id"], id) {
			continue
		}
		if _, hasError := value["error"]; hasError {
			return nativeRecoveryRejected, nil
		}
		result, _ := value["result"].(map[string]any)
		if result == nil {
			result = map[string]any{}
		}
		return nativeRecoveryAccepted, result
	}
	return nativeRecoveryAmbiguous, nil
}

type nativeRecoveryRequestOutcome uint8

const (
	nativeRecoveryRejected nativeRecoveryRequestOutcome = iota
	nativeRecoveryAccepted
	nativeRecoveryAmbiguous
)

func nativeJSONIDMatches(value any, expected uint64) bool {
	number, ok := value.(json.Number)
	if !ok {
		return false
	}
	parsed, err := number.Int64()
	return err == nil && parsed >= 0 && uint64(parsed) == expected
}

func nativeWebSocketAccept(key string) string {
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func headerHasToken(value, token string) bool {
	for _, item := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(item), token) {
			return true
		}
	}
	return false
}
