package kiro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	kiroLiveSequenceField   = "sequence_number"
	kiroLiveResponseIDField = "response_id"
	kiroLiveCreatedAtField  = "created_at"
)

type kiroLiveState struct {
	requestID        uint64
	responseID       string
	chatCompletionID string
	messageItemID    string
	streamModel      string
	profileName      string
	createdAt        uint64
	sequence         uint64
	messageOpen      bool
	chatStarted      bool
	turn             kiroTurnState
}

func newKiroLiveState(requestID uint64, requestedModel, profileName string) *kiroLiveState {
	responseID := fmt.Sprintf("resp_kiro_%d", requestID)
	model := strings.TrimSpace(requestedModel)
	if model == "" {
		model = "kiro-cli"
	}
	return &kiroLiveState{
		requestID:        requestID,
		responseID:       responseID,
		chatCompletionID: "chatcmpl_" + responseID,
		messageItemID:    fmt.Sprintf("msg_kiro_%d_0", requestID),
		streamModel:      model,
		profileName:      profileName,
		createdAt:        uint64(time.Now().Unix()),
		turn:             kiroTurnState{toolLabels: make(map[string]toolLabel)},
	}
}

func (state *kiroLiveState) start(route runtimeRoute) []byte {
	if route.kind == routeChat {
		state.chatStarted = true
		return kiroChatChunk(state.chatCompletionID, state.streamModel, map[string]any{"role": kiroRoleAssistant}, "")
	}
	return kiroResponsesEvent("response.created", map[string]any{
		"type":                 "response.created",
		kiroLiveSequenceField:  state.sequence,
		kiroLiveCreatedAtField: state.createdAt,
		"response":             map[string]any{"id": state.responseID},
	})
}

func (state *kiroLiveState) observe(route runtimeRoute, envelope acpEnvelope) [][]byte {
	if envelope.Method != "session/update" || len(envelope.Params) == 0 {
		return nil
	}
	var params map[string]any
	if json.Unmarshal(envelope.Params, &params) != nil {
		return nil
	}
	update, _ := params["update"].(map[string]any)
	if update == nil {
		return nil
	}
	assistantBefore := len(state.turn.assistantText)
	reasoningBefore := len(state.turn.reasoningText)
	state.turn.applyUpdate(update)
	assistantDelta := state.turn.assistantText[assistantBefore:]
	reasoningDelta := state.turn.reasoningText[reasoningBefore:]

	var chunks [][]byte
	if route.kind == routeChat {
		if reasoningDelta != "" {
			chunks = append(chunks, kiroChatChunk(
				state.chatCompletionID, state.streamModel,
				map[string]any{"reasoning_content": reasoningDelta}, "",
			))
			state.chatStarted = true
		}
		if assistantDelta != "" {
			chunks = append(chunks, kiroChatChunk(
				state.chatCompletionID, state.streamModel,
				map[string]any{kiroFieldContent: assistantDelta}, "",
			))
			state.chatStarted = true
		}
		return chunks
	}
	if assistantDelta == "" {
		return nil
	}
	if !state.messageOpen {
		state.sequence++
		chunks = append(chunks, kiroResponsesEvent("response.output_item.added", map[string]any{
			"type":                  "response.output_item.added",
			kiroLiveSequenceField:   state.sequence,
			kiroLiveResponseIDField: state.responseID,
			"item": map[string]any{
				"id": state.messageItemID, "type": "message",
				"role": kiroRoleAssistant, kiroFieldContent: []any{},
			},
		}))
		state.messageOpen = true
	}
	state.sequence++
	chunks = append(chunks, kiroResponsesEvent("response.output_text.delta", map[string]any{
		"type":                  "response.output_text.delta",
		kiroLiveSequenceField:   state.sequence,
		kiroLiveCreatedAtField:  state.createdAt,
		kiroLiveResponseIDField: state.responseID,
		"delta":                 assistantDelta,
	}))
	return chunks
}

func (state *kiroLiveState) finishResponse(route runtimeRoute, response map[string]any) [][]byte {
	if route.kind == routeChat {
		finish := kiroChatFinishReason(response)
		return [][]byte{
			kiroChatChunk(state.chatCompletionID, "", map[string]any{}, finish),
			[]byte("data: [DONE]\n\n"),
		}
	}
	var chunks [][]byte
	if state.messageOpen {
		state.sequence++
		chunks = append(chunks, kiroResponsesEvent("response.output_item.done", map[string]any{
			"type":                  "response.output_item.done",
			kiroLiveSequenceField:   state.sequence,
			kiroLiveResponseIDField: state.responseID,
			"item": map[string]any{
				"id": state.messageItemID, "type": "message", "role": kiroRoleAssistant,
				kiroFieldContent: []any{map[string]any{
					"type": "output_text", "text": state.turn.assistantText,
				}},
			},
		}))
	}
	state.sequence++
	eventType := "response.completed"
	switch responseString(response["status"], "") {
	case "failed":
		eventType = "response.failed"
	case "incomplete":
		eventType = "response.incomplete"
	}
	chunks = append(chunks, kiroResponsesEvent(eventType, map[string]any{
		"type":                 eventType,
		kiroLiveSequenceField:  state.sequence,
		kiroLiveCreatedAtField: state.createdAt,
		"response":             response,
	}))
	chunks = append(chunks, []byte("data: [DONE]\r\n\r\n"))
	return chunks
}

func kiroResponsesEvent(event string, value any) []byte {
	content, _ := json.Marshal(value)
	var output bytes.Buffer
	fmt.Fprintf(&output, "event: %s\r\ndata: %s\r\n\r\n", event, content)
	return output.Bytes()
}

func kiroChatChunk(id, model string, delta map[string]any, finishReason string) []byte {
	choice := map[string]any{"index": 0, "delta": delta}
	if finishReason != "" {
		choice["finish_reason"] = finishReason
	}
	value := map[string]any{
		"id": id, "object": "chat.completion.chunk",
		"choices": []any{choice},
	}
	if model != "" {
		value["model"] = model
	}
	content, _ := json.Marshal(value)
	return append(append([]byte("data: "), content...), []byte("\n\n")...)
}
