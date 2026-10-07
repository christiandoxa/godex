package routing

import (
	"context"
	"net/http"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04357ObserveRecordsTerminalResponsesTokenUsage(t *testing.T) {
	recorder := &routingMarkerRecorder{}
	router, err := NewRouter(Config{
		Activity: recorder,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "main", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("{\"type\":\"response.completed\",\"response\":{\"id\":\"resp-1\",\"usage\":{\"input_tokens\":110,\"input_tokens_details\":{\"cached_tokens\":25},\"output_tokens\":44,\"output_tokens_details\":{\"reasoning_tokens\":9}}}}")
	if err := router.Observe(t.Context(), "main", make(http.Header), payload, false); err != nil {
		t.Fatal(err)
	}
	if len(recorder.events) != 1 {
		t.Fatalf("token events = %#v", recorder.events)
	}
	event := recorder.events[0]
	if event.Kind != "token_usage" || event.AccountID != "main" ||
		event.Fields["input_tokens"] != "110" ||
		event.Fields["cached_input_tokens"] != "25" ||
		event.Fields["output_tokens"] != "44" ||
		event.Fields["reasoning_tokens"] != "9" ||
		event.Fields["profile"] != "main" {
		t.Fatalf("token event = %#v", event)
	}
}

func TestProdex04357ObserveIgnoresNonterminalUsageProgressButAcceptsChatAliases(t *testing.T) {
	recorder := &routingMarkerRecorder{}
	router, err := NewRouter(Config{
		Activity: recorder,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "main", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	progress := []byte("{\"type\":\"response.output_text.delta\",\"usage\":{\"input_tokens\":999}}")
	if err := router.Observe(t.Context(), "main", make(http.Header), progress, false); err != nil {
		t.Fatal(err)
	}
	buffered := []byte("{\"id\":\"chat-1\",\"usage\":{\"prompt_tokens\":20,\"prompt_tokens_details\":{\"cached_tokens\":5},\"completion_tokens\":7,\"completion_tokens_details\":{\"reasoning_tokens\":2}}}")
	if err := router.Observe(t.Context(), "main", make(http.Header), buffered, false); err != nil {
		t.Fatal(err)
	}
	if len(recorder.events) != 1 {
		t.Fatalf("token events = %#v", recorder.events)
	}
	event := recorder.events[0]
	if event.Fields["input_tokens"] != "20" || event.Fields["cached_input_tokens"] != "5" ||
		event.Fields["output_tokens"] != "7" || event.Fields["reasoning_tokens"] != "2" {
		t.Fatalf("chat token alias event = %#v", event)
	}
}
