package routing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type affinityBindingRemover interface {
	Remove(context.Context, []string) error
}

func (store *affinityStore) forgetPreviousResponse(
	ctx context.Context,
	responseID, accountID string,
) error {
	responseID = strings.TrimSpace(responseID)
	if responseID == "" || accountID == "" {
		return nil
	}
	entries := (affinityKeys{previous: responseID}).entries()
	if len(entries) != 1 {
		return nil
	}
	key := entries[0].Key
	store.mu.Lock()
	defer store.mu.Unlock()
	if current, ok := store.values[key]; ok && current.accountID != accountID {
		return nil
	}
	if remover, ok := store.repository.(affinityBindingRemover); ok && store.writesEnabled() {
		if err := remover.Remove(ctx, []string{key}); err != nil {
			return fmt.Errorf("remove dead previous-response binding: %w", err)
		}
	}
	now := time.Now()
	if store.clock != nil {
		now = store.clock()
	}
	store.markContinuationDeadLocked("response", key, now)
	delete(store.values, key)
	return nil
}

func (router *Router) handleInvalidWebSocketPreviousResponse(
	ctx context.Context,
	response *proxymodel.Response,
	metadata websocketRequestMetadata,
	account proxymodel.Account,
	ownerMatches bool,
) (*proxymodel.Response, error) {
	payload := readWebSocketPrecommitTextPayload(response)
	closeWebSocketRoutingResponse(response)
	if err := router.affinity.forgetPreviousResponse(ctx, metadata.previousResponseID, account.ID); err != nil {
		return nil, err
	}
	recoverySignal := metadata.previousResponseID != "" && metadata.sessionID != "" && ownerMatches
	if !recoverySignal {
		return websocketTextPayloadResponse(payload, http.StatusBadRequest), nil
	}
	return websocketTextPayloadResponse(
		translateInvalidPreviousResponseSignal(payload), http.StatusBadRequest,
	), nil
}

func translateInvalidPreviousResponseSignal(payload []byte) []byte {
	var value map[string]any
	if json.Unmarshal(payload, &value) != nil {
		return payload
	}
	errorValue, ok := value["error"].(map[string]any)
	if !ok {
		return payload
	}
	if _, exists := errorValue["code"]; !exists {
		errorValue["code"] = "previous_response_not_found"
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return payload
	}
	return encoded
}

func websocketTextPayloadResponse(payload []byte, status int) *proxymodel.Response {
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	return &proxymodel.Response{
		StatusCode:          status,
		Header:              headers,
		Body:                io.NopCloser(bytes.NewReader(payload)),
		FirstEventCommitted: true,
	}
}
