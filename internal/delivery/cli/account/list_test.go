package account

import (
	"bytes"
	"context"
	"errors"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
)

type testAccounts struct {
	listed   []accountentity.Account
	selected string
	removed  string
}

func (accounts testAccounts) List(context.Context) ([]accountentity.Account, error) {
	return accounts.listed, nil
}

func (accounts *testAccounts) Current(context.Context) (accountentity.Account, error) {
	if len(accounts.listed) == 0 {
		return accountentity.Account{}, errors.New("no active account")
	}
	return accounts.listed[0], nil
}

func (accounts *testAccounts) SetActive(_ context.Context, selector string) (accountentity.Account, error) {
	accounts.selected = selector
	return accountentity.Account{Name: selector}, nil
}

func (accounts *testAccounts) Remove(_ context.Context, selector string) (accountentity.Account, error) {
	accounts.removed = selector
	return accountentity.Account{Name: selector}, nil
}

type failOnceWriter struct {
	err    error
	failed bool
	bytes.Buffer
}

func (writer *failOnceWriter) Write(data []byte) (int, error) {
	if !writer.failed {
		writer.failed = true
		return 0, writer.err
	}
	return writer.Buffer.Write(data)
}

func TestListReturnsIntermediateOutputError(t *testing.T) {
	errSynthetic := errors.New("synthetic output failure")
	writer := &failOnceWriter{err: errSynthetic}
	accounts := &testAccounts{listed: []accountentity.Account{{
		Name:    "name\n",
		Email:   "email@example.com",
		ID:      "synthetic-id",
		Enabled: true,
	}}}

	err := List(context.Background(), accounts, writer, nil)
	if !errors.Is(err, errSynthetic) {
		t.Fatalf("list error = %v", err)
	}
}

func TestAccountCommandsRenderAndRoute(t *testing.T) {
	accounts := &testAccounts{}
	var output bytes.Buffer
	if err := List(context.Background(), accounts, &output, nil); err != nil {
		t.Fatal(err)
	}
	if output.String() != "No accounts. Run `godex login`.\n" {
		t.Fatalf("empty list output = %q", output.String())
	}
	output.Reset()
	if err := Run(context.Background(), accounts, &output, []string{"use", "work"}); err != nil {
		t.Fatal(err)
	}
	if accounts.selected != "work" || output.String() != "Next launch will start with work.\n" {
		t.Fatalf("use result = selector %q, output %q", accounts.selected, output.String())
	}
	output.Reset()
	if err := Run(context.Background(), accounts, &output, []string{"remove", "work"}); err != nil {
		t.Fatal(err)
	}
	if accounts.removed != "work" || output.String() != "Removed work.\n" {
		t.Fatalf("remove result = selector %q, output %q", accounts.removed, output.String())
	}
}

func TestAccountCommandRejectsInvalidArguments(t *testing.T) {
	accounts := &testAccounts{}
	var output bytes.Buffer
	for _, arguments := range [][]string{nil, {"unknown"}, {"list", "extra"}, {"use"}, {"remove"}} {
		if err := Run(context.Background(), accounts, &output, arguments); err == nil {
			t.Fatalf("arguments %#v unexpectedly succeeded", arguments)
		}
	}
}

type enabledAccounts struct {
	testAccounts
	enabled bool
}

func (accounts *enabledAccounts) SetEnabled(_ context.Context, selector string, enabled bool) (accountentity.Account, error) {
	accounts.enabled = enabled
	return accountentity.Account{Name: selector, Enabled: enabled}, nil
}
func TestEnableAndDisableDelivery(t *testing.T) {
	accounts := &enabledAccounts{}
	var out bytes.Buffer
	for _, action := range []string{"enable", "disable"} {
		if err := Run(context.Background(), accounts, &out, []string{action, "work"}); err != nil {
			t.Fatal(err)
		}
		if accounts.enabled != (action == "enable") {
			t.Fatal("enablement flag was lost")
		}
	}
	if err := Run(context.Background(), accounts, &out, []string{"disable"}); err == nil {
		t.Fatal("missing selector accepted")
	}
}
