package providerkey

import (
	"reflect"
	"strings"
	"testing"
)

func TestProviderKeysMatchProdexPrecedence(t *testing.T) {
	for provider, names := range map[string][2]string{
		"anthropic": {"ANTHROPIC_API_KEYS", "ANTHROPIC_API_KEY"},
		"deepseek":  {"DEEPSEEK_API_KEYS", "DEEPSEEK_API_KEY"},
	} {
		t.Run(provider, func(t *testing.T) {
			env := map[string]string{names[0]: "one, two", names[1]: "single"}
			source := &Source{lookup: func(name string) (string, bool) { value, ok := env[name]; return value, ok }}
			keys, err := source.APIKeys(provider, "explicit")
			if err != nil || !reflect.DeepEqual(keys, []string{"explicit"}) {
				t.Fatalf("explicit keys = %#v, err=%v", keys, err)
			}
			keys, err = source.APIKeys(provider, "")
			if err != nil || !reflect.DeepEqual(keys, []string{"one", "two"}) {
				t.Fatalf("plural keys = %#v, err=%v", keys, err)
			}
			delete(env, names[0])
			keys, err = source.APIKeys(provider, "")
			if err != nil || !reflect.DeepEqual(keys, []string{"single"}) {
				t.Fatalf("single keys = %#v, err=%v", keys, err)
			}
		})
	}
}

func TestProviderKeysRejectEmptyPluralAndWhitespaceSingle(t *testing.T) {
	source := &Source{lookup: func(name string) (string, bool) {
		if name == "DEEPSEEK_API_KEYS" {
			return " , ; \n ", true
		}
		if name == "DEEPSEEK_API_KEY" {
			return "fallback", true
		}
		return "", false
	}}
	if _, err := source.APIKeys("deepseek", ""); err == nil || !strings.Contains(err.Error(), "DEEPSEEK_API_KEYS cannot be empty") {
		t.Fatalf("empty plural error = %v", err)
	}
	for _, value := range []string{"", "bad key", "bad\tkey", "secret　"} {
		if _, err := single(value, "--api-key"); err == nil {
			t.Fatalf("invalid single key %q accepted", value)
		}
	}
}

func TestProviderKeyListAcceptsReferenceSeparators(t *testing.T) {
	got := list(" one, two;three\nfour ")
	want := []string{"one", "two", "three", "four"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("keys = %#v", got)
	}
	if got := list("key with space,second"); !reflect.DeepEqual(got, []string{"key with space", "second"}) {
		t.Fatalf("plural whitespace keys = %#v", got)
	}
}
