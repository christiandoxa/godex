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

func TestGeminiProviderKeysMatchProdexPrecedence(t *testing.T) {
	for _, test := range []struct {
		name string
		env  map[string]string
		want []string
	}{
		{name: "Gemini plural", env: map[string]string{"GEMINI_API_KEYS": "gemini-one", "GOOGLE_API_KEYS": "google-one"}, want: []string{"gemini-one"}},
		{name: "Google plural", env: map[string]string{"GOOGLE_API_KEYS": "google-one", "GEMINI_API_KEY": "gemini-single"}, want: []string{"google-one"}},
		{name: "Gemini single", env: map[string]string{"GEMINI_API_KEY": "gemini-single", "GOOGLE_API_KEY": "google-single"}, want: []string{"gemini-single"}},
		{name: "Google single", env: map[string]string{"GOOGLE_API_KEY": "google-single"}, want: []string{"google-single"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := &Source{lookup: func(name string) (string, bool) { value, ok := test.env[name]; return value, ok }}
			keys, err := source.APIKeys("gemini", "")
			if err != nil || !reflect.DeepEqual(keys, test.want) {
				t.Fatalf("Gemini keys = %#v, err=%v; want %#v", keys, err, test.want)
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
	gemini := &Source{lookup: func(name string) (string, bool) {
		if name == "GEMINI_API_KEYS" {
			return " , ; \n ", true
		}
		if name == "GOOGLE_API_KEYS" {
			return "fallback", true
		}
		return "", false
	}}
	if _, err := gemini.APIKeys("gemini", ""); err == nil || !strings.Contains(err.Error(), "GEMINI_API_KEYS cannot be empty") {
		t.Fatalf("empty Gemini plural error = %v", err)
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
