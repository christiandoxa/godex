package claude

import (
	"reflect"
	"strings"
	"testing"
)

func TestResolveRuntimeAPIKeysMatchesProdexPrecedence(t *testing.T) {
	environment := map[string]string{
		anthropicAPIKeysEnv: "plural-one, plural-two",
		anthropicAPIKeyEnv:  "single",
	}
	lookup := func(name string) (string, bool) {
		value, ok := environment[name]
		return value, ok
	}
	keys, err := resolveRuntimeAPIKeys("explicit", lookup)
	if err != nil || !reflect.DeepEqual(keys, []string{"explicit"}) {
		t.Fatalf("explicit keys = %#v, err=%v", keys, err)
	}
	keys, err = resolveRuntimeAPIKeys("", lookup)
	if err != nil || !reflect.DeepEqual(keys, []string{"plural-one", "plural-two"}) {
		t.Fatalf("plural keys = %#v, err=%v", keys, err)
	}
	delete(environment, anthropicAPIKeysEnv)
	keys, err = resolveRuntimeAPIKeys("", lookup)
	if err != nil || !reflect.DeepEqual(keys, []string{"single"}) {
		t.Fatalf("single keys = %#v, err=%v", keys, err)
	}
}

func TestRuntimeAPIKeyListMatchesProdexSeparators(t *testing.T) {
	got := runtimeAPIKeyList(" one, two;three\nfour ")
	want := []string{"one", "two", "three", "four"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("keys = %#v, want %#v", got, want)
	}
	// Prodex trims list entries but does not apply single-key whitespace validation.
	if got := runtimeAPIKeyList("key with space,second"); !reflect.DeepEqual(got, []string{"key with space", "second"}) {
		t.Fatalf("whitespace list = %#v", got)
	}
}

func TestResolveRuntimeAPIKeysRejectsEmptyPluralAndInvalidSingle(t *testing.T) {
	lookup := func(name string) (string, bool) {
		if name == anthropicAPIKeysEnv {
			return " , ; \n ", true
		}
		return "fallback", true
	}
	if _, err := resolveRuntimeAPIKeys("", lookup); err == nil || !strings.Contains(err.Error(), "cannot be empty") {
		t.Fatalf("empty plural error = %v", err)
	}
	for _, fixture := range []struct{ value, name string }{{"", "--api-key"}, {"bad key", "--api-key"}, {"bad\tkey", anthropicAPIKeyEnv}} {
		if _, err := runtimeSingleAPIKey(fixture.value, fixture.name); err == nil {
			t.Fatalf("invalid key %#v unexpectedly accepted", fixture.value)
		}
	}
}

func TestRuntimeSingleAPIKeyRejectsUnicodeWhitespace(t *testing.T) {
	for _, value := range []string{"prefix suffix", " secret", "secret　"} {
		if _, err := runtimeSingleAPIKey(value, "--api-key"); err == nil ||
			!strings.Contains(err.Error(), "must not contain whitespace") {
			t.Fatalf("Unicode-whitespace API key %q error = %v", value, err)
		}
	}
}
