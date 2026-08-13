package runtime

import "testing"

func TestParseRunArguments(t *testing.T) {
	selector, arguments, err := parseRunArguments([]string{"--account", "work", "--", "--model", "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	if selector != "work" || len(arguments) != 2 || arguments[0] != "--model" {
		t.Fatalf("selector=%q arguments=%#v", selector, arguments)
	}
}

func TestParseRunArgumentsPassesCodexFlagsWithoutSeparator(t *testing.T) {
	selector, arguments, err := parseRunArguments([]string{"--model", "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	if selector != "" || len(arguments) != 2 {
		t.Fatalf("selector=%q arguments=%#v", selector, arguments)
	}
}

func TestParseRunArgumentsRequiresAccountSelector(t *testing.T) {
	if _, _, err := parseRunArguments([]string{"--account"}); err == nil {
		t.Fatal("missing account selector unexpectedly accepted")
	}
}
