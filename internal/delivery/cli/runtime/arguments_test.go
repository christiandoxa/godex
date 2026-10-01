package runtime

import "testing"

func TestParseRunArguments(t *testing.T) {
	selector, arguments, err := parseRunArguments([]string{"--account", "work", "--", "--model", "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	if selector.Account != "work" || selector.Profile != "" || len(arguments) != 2 || arguments[0] != "--model" {
		t.Fatalf("selection=%+v arguments=%#v", selector, arguments)
	}
}

func TestParseRunArgumentsPassesCodexFlagsWithoutSeparator(t *testing.T) {
	selector, arguments, err := parseRunArguments([]string{"--model", "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	if !selector.Empty() || len(arguments) != 2 {
		t.Fatalf("selection=%+v arguments=%#v", selector, arguments)
	}
}

func TestParseRunArgumentsRequiresAccountSelector(t *testing.T) {
	if _, _, err := parseRunArguments([]string{"--account"}); err == nil {
		t.Fatal("missing account selector unexpectedly accepted")
	}
}

func TestParseRunArgumentsSupportsProfileSelectors(t *testing.T) {
	for _, arguments := range [][]string{{"--profile", "work", "exec"}, {"-p=work", "exec"}} {
		selection, codexArguments, err := parseRunArguments(arguments)
		if err != nil {
			t.Fatal(err)
		}
		if selection.Profile != "work" || selection.Account != "" || len(codexArguments) != 1 {
			t.Fatalf("selection/arguments = %+v / %#v", selection, codexArguments)
		}
	}
	if _, _, err := parseRunArguments([]string{"--account", "one", "--profile", "two"}); err == nil {
		t.Fatal("account/profile conflict unexpectedly accepted")
	}
}

func TestParseRunArgumentsSupportsProfileSelection(t *testing.T) {
	selection, arguments, err := parseRunArguments([]string{"--profile", "work", "--", "exec", "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if selection.Profile != "work" || selection.Account != "" || len(arguments) != 2 || arguments[0] != "exec" {
		t.Fatalf("selection=%#v arguments=%#v", selection, arguments)
	}
	if _, _, err := parseRunArguments([]string{"--account", "one", "--profile", "two"}); err == nil {
		t.Fatal("account and profile selectors unexpectedly combined")
	}
}
