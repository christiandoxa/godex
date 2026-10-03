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

type nativeAntigravityFake struct {
	home string
	args []string
	runs int
	err  error
}

type nativeSessionLockerFake struct {
	home       string
	released   bool
	lockErr    error
	releaseErr error
}

func (locker *nativeSessionLockerFake) LockCodexSessionsForChild(_ context.Context, home string) (func() error, error) {
	locker.home = home
	if locker.lockErr != nil {
		return nil, locker.lockErr
	}
	return func() error {
		locker.released = true
		return locker.releaseErr
	}, nil
}

func (f *nativeAntigravityFake) RunWithCodexHome(_ context.Context, home string, args []string) error {
	f.runs++
	f.home = home
	f.args = append([]string(nil), args...)
	return f.err
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
		if err := NewNative(accounts, process, nil).Run(context.Background(), input); !errors.Is(err, process.err) {
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

func TestNativeAntigravityLoginRunsGlobalCLIAndPropagatesFailure(t *testing.T) {
	processErr := errors.New("synthetic Antigravity failure")
	process := &nativeAntigravityFake{err: processErr}
	native := NewNative(nil, nil, process)
	native.SetAntigravityCodexHome("/synthetic/shared-codex")
	locker := &nativeSessionLockerFake{}
	native.SetAntigravitySessionLocker(locker)
	if err := native.RunAntigravityLogin(context.Background()); !errors.Is(err, processErr) {
		t.Fatalf("Antigravity login error = %v", err)
	}
	if process.home != "/synthetic/shared-codex" || !reflect.DeepEqual(process.args, []string{"auth", "login"}) {
		t.Fatalf("Antigravity login home/args = %q / %#v", process.home, process.args)
	}
	if locker.home != process.home || !locker.released {
		t.Fatalf("session lock home/released = %q/%t", locker.home, locker.released)
	}
	if err := NewNative(nil, nil, process).RunAntigravityLogin(context.Background()); err == nil {
		t.Fatal("missing Antigravity CODEX_HOME unexpectedly accepted")
	}
	if err := NewNative(nil, nil, nil).RunAntigravityLogin(context.Background()); err == nil {
		t.Fatal("unconfigured Antigravity CLI unexpectedly accepted")
	}
	if err := NewNative(nil, nil, process).RunAntigravityLogin(context.Background()); err == nil {
		t.Fatal("missing Antigravity session locker unexpectedly accepted")
	}
	lockErr := errors.New("synthetic session lock failure")
	locker = &nativeSessionLockerFake{lockErr: lockErr}
	native.SetAntigravitySessionLocker(locker)
	if err := native.RunAntigravityLogin(context.Background()); !errors.Is(err, lockErr) || process.runs != 1 {
		t.Fatalf("session lock error = %v, child runs = %d", err, process.runs)
	}
}

func (f *nativeAccountsFake) AcquireProfileMutation(ctx context.Context, id string) (func() error, error) {
	return f.AcquireProfiles(ctx, []string{id})
}
