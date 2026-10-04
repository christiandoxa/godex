package openai

import (
	"io"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type websocketMessageSession struct {
	connection io.ReadWriteCloser
	accountID  string
	home       string
	turnState  string
	completed  time.Time
}

func (transport *Transport) takeWebSocketMessageSession(
	sessionID uint64,
	account proxymodel.Account,
	turnStateOverride string,
) websocketMessageSession {
	if sessionID == 0 {
		return websocketMessageSession{}
	}
	transport.websocketMessageMu.Lock()
	session, ok := transport.websocketMessageSessions[sessionID]
	delete(transport.websocketMessageSessions, sessionID)
	transport.websocketMessageMu.Unlock()
	if !ok {
		return websocketMessageSession{}
	}
	if session.accountID != account.ID || session.home != account.Home ||
		(turnStateOverride != "" && session.turnState != turnStateOverride) {
		_ = session.connection.Close()
		return websocketMessageSession{}
	}
	return session
}

func (transport *Transport) recycleWebSocketMessageSession(
	sessionID uint64,
	account proxymodel.Account,
	connection io.ReadWriteCloser,
	turnState string,
) {
	if sessionID == 0 {
		_ = connection.Close()
		return
	}
	transport.websocketMessageMu.Lock()
	old, exists := transport.websocketMessageSessions[sessionID]
	keep := !transport.websocketMessageClosed
	if keep {
		transport.websocketMessageSessions[sessionID] = websocketMessageSession{
			connection: connection, accountID: account.ID, home: account.Home,
			turnState: turnState, completed: time.Now(),
		}
	}
	transport.websocketMessageMu.Unlock()
	if exists {
		_ = old.connection.Close()
	}
	if !keep {
		_ = connection.Close()
	}
}

func (transport *Transport) CloseWebSocketSession(sessionID uint64) {
	transport.websocketMessageMu.Lock()
	session, ok := transport.websocketMessageSessions[sessionID]
	delete(transport.websocketMessageSessions, sessionID)
	transport.websocketMessageMu.Unlock()
	if ok {
		_ = session.connection.Close()
	}
}

func (transport *Transport) closeWebSocketMessageSessions() {
	transport.websocketMessageMu.Lock()
	transport.websocketMessageClosed = true
	sessions := transport.websocketMessageSessions
	transport.websocketMessageSessions = make(map[uint64]websocketMessageSession)
	transport.websocketMessageMu.Unlock()
	for _, session := range sessions {
		_ = session.connection.Close()
	}
}
