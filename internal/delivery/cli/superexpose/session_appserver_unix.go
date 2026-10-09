//go:build linux

package superexpose

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"
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
		if first&0x70 != 0 || masked {
			return "", errors.New("invalid app-server websocket frame")
		}
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
			if !fin || length == 1 || length > 125 {
				return "", errors.New("invalid app-server websocket close frame")
			}
			return "", errors.New("app-server websocket closed")
		case 0x9:
			if !fin || length > 125 {
				return "", errors.New("invalid app-server websocket ping frame")
			}
			if err := socket.writeFrame(0xA, payload); err != nil {
				return "", err
			}
			continue
		case 0xA:
			if !fin || length > 125 {
				return "", errors.New("invalid app-server websocket pong frame")
			}
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
