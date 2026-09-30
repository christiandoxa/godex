package account

import (
	"context"
	"errors"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
)

type fakeAccounts struct {
	accounts []accountentity.Account
	selected string
	removed  string
	err      error
}

func (fake *fakeAccounts) List(context.Context) ([]accountentity.Account, error) {
	return fake.accounts, fake.err
}

func (fake *fakeAccounts) Current(context.Context) (accountentity.Account, error) {
	if fake.err != nil {
		return accountentity.Account{}, fake.err
	}
	return accountentity.Account{Name: "current"}, nil
}

func (fake *fakeAccounts) SetActive(_ context.Context, selector string) (accountentity.Account, error) {
	fake.selected = selector
	return accountentity.Account{Name: selector}, fake.err
}

func (fake *fakeAccounts) Remove(_ context.Context, selector string) (accountentity.Account, error) {
	fake.removed = selector
	return accountentity.Account{Name: selector}, fake.err
}

func TestAccountUseCasesDelegateWithoutChangingResults(t *testing.T) {
	fake := &fakeAccounts{accounts: []accountentity.Account{{ID: "one"}}}

	listed, err := List(context.Background(), fake)
	if err != nil || len(listed) != 1 || listed[0].ID != "one" {
		t.Fatalf("list = %#v, err = %v", listed, err)
	}
	if _, err := Use(context.Background(), fake, "work"); err != nil || fake.selected != "work" {
		t.Fatalf("use selector = %q, err = %v", fake.selected, err)
	}
	if _, err := Remove(context.Background(), fake, "work"); err != nil || fake.removed != "work" {
		t.Fatalf("remove selector = %q, err = %v", fake.removed, err)
	}
}

func TestAccountUseCasesReturnStoreErrors(t *testing.T) {
	errSynthetic := errors.New("synthetic failure")
	fake := &fakeAccounts{err: errSynthetic}
	if _, err := List(context.Background(), fake); !errors.Is(err, errSynthetic) {
		t.Fatalf("list error = %v", err)
	}
	if _, err := Use(context.Background(), fake, "work"); !errors.Is(err, errSynthetic) {
		t.Fatalf("use error = %v", err)
	}
	if _, err := Remove(context.Background(), fake, "work"); !errors.Is(err, errSynthetic) {
		t.Fatalf("remove error = %v", err)
	}
}
