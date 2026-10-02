package kiro

import (
	"context"
	"strconv"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestKiroPreviousResponseIDReplaysConversation(t *testing.T) {
	transport, prompts := newConversationKiroTransport(t, func(call int) string {
		if call == 0 {
			return "first answer"
		}
		return "second answer"
	})

	first, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: "POST", Path: runtimeMountPath + "/responses",
		Body: []byte(`{"input":"first question"}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	first.Body.Close()

	second, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: "POST", Path: runtimeMountPath + "/responses",
		Body: []byte(`{"previous_response_id":"resp_kiro_0","input":"second question"}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	second.Body.Close()

	if len(*prompts) != 2 {
		t.Fatalf("prompts = %#v", *prompts)
	}
	for _, expected := range []string{"first question", "Assistant:\nfirst answer", "second question"} {
		if !strings.Contains((*prompts)[1], expected) {
			t.Fatalf("continuation prompt missing %q: %q", expected, (*prompts)[1])
		}
	}
}

func TestKiroToolOutputFindsLatestConversationByCallID(t *testing.T) {
	transport, prompts := newConversationKiroTransport(t, func(call int) string {
		return "answer"
	})
	firstBody := `{"input":[{"type":"message","role":"user","content":"run tool"},{"type":"function_call","call_id":"call_1","name":"lookup","arguments":{"q":"x"}}]}`
	first, err := transport.Execute(context.Background(), proxymodel.Request{Method: "POST", Path: runtimeMountPath + "/responses", Body: []byte(firstBody)}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	first.Body.Close()

	secondBody := `{"input":[{"type":"function_call_output","call_id":"call_1","output":"tool result"}]}`
	second, err := transport.Execute(context.Background(), proxymodel.Request{Method: "POST", Path: runtimeMountPath + "/responses", Body: []byte(secondBody)}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	second.Body.Close()
	if len(*prompts) != 2 || !strings.Contains((*prompts)[1], "run tool") || !strings.Contains((*prompts)[1], "tool result") {
		t.Fatalf("tool replay prompt = %#v", *prompts)
	}
}

func TestKiroConversationStoreEvictsOldestPerScope(t *testing.T) {
	store := newKiroConversationStore()
	for index := 0; index <= kiroConversationsPerScope; index++ {
		store.insert("scope", "resp_"+strconv.Itoa(index), "history", nil)
	}
	if _, ok := store.history("scope", "resp_0"); ok {
		t.Fatal("oldest scoped conversation was not evicted")
	}
	if _, ok := store.history("scope", "resp_"+strconv.Itoa(kiroConversationsPerScope)); !ok {
		t.Fatal("newest scoped conversation was evicted")
	}
}

func newConversationKiroTransport(t *testing.T, answer func(int) string) (*RuntimeTransport, *[]string) {
	t.Helper()
	home := writeKiroRuntimeHome(t)
	source := NewSource()
	prompts := make([]string, 0, 2)
	call := 0
	source.acp = func(_ context.Context, _, model, _, prompt string) (acpTurn, error) {
		prompts = append(prompts, prompt)
		turn := fixtureACPTurn(model, answer(call), "")
		call++
		return turn, nil
	}
	transport, err := source.NewRuntimeTransport(context.Background(), home, "kiro-work")
	if err != nil {
		t.Fatal(err)
	}
	return transport, &prompts
}
