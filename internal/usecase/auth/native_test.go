package auth

import (
	"context"
	"errors"
	"reflect"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	authmodel "github.com/christiandoxa/godex/internal/model/auth"
)

type nativeAccountsFake struct{ released bool }

func (*nativeAccountsFake) Current(context.Context) (accountentity.Account, error) {
	return accountentity.Account{ID: "active"}, nil
}
func (*nativeAccountsFake) Resolve(_ context.Context, selector string) (accountentity.Account, error) {
	return accountentity.Account{ID: selector}, nil
}
func (*nativeAccountsFake) CodexHome(id string) string { return id }
func (f *nativeAccountsFake) AcquireProfiles(context.Context, []string) (func() error, error) {
	return func() error { f.released = true; return nil }, nil
}

type nativeProcessFake struct {
	home string
	args []string
	err  error
}

func (f *nativeProcessFake) Run(_ context.Context, home string, args []string) error {
	f.home = home
	f.args = args
	return f.err
}
func TestNativeAuthUsesActiveOrExplicitAccountWithoutQuotaOrRotation(t *testing.T) {
	for _, input := range []authmodel.Command{{}, {Selector: "chosen", Logout: true}} {
		accounts := &nativeAccountsFake{}
		process := &nativeProcessFake{err: errors.New("child failure")}
		if err := NewNative(accounts, process).Run(context.Background(), input); !errors.Is(err, process.err) {
			t.Fatalf("exit error = %v", err)
		}
		home, args := "active", []string{"login", "status"}
		if input.Logout {
			home, args = "chosen", []string{"logout"}
		}
		if process.home != home || !reflect.DeepEqual(process.args, args) || !accounts.released {
			t.Fatalf("native call = %#v", process)
		}
	}
}
