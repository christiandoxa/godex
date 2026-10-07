package auth

import (
	"context"
	"errors"
)

type selectedLoginSnapshotReader interface {
	ReadAuthSnapshot(context.Context, string) ([]byte, error)
}

type selectedLoginRunner interface {
	Run(context.Context, string, []string) error
}

func (login *Login) RunSelected(ctx context.Context, deviceAuth bool) (authJSON []byte, err error) {
	if login == nil || login.accounts == nil || login.codex == nil {
		return nil, errors.New("selected profile login is not configured")
	}
	reader, ok := login.codex.(selectedLoginSnapshotReader)
	if !ok {
		return nil, errors.New("selected profile auth snapshot support is not configured")
	}
	stagedHome, err := login.accounts.CreateStagedHome()
	if err != nil {
		return nil, err
	}
	defer func() {
		if cleanupErr := login.removeAll(stagedHome); cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
		}
	}()
	if _, err := login.codex.Login(ctx, stagedHome, deviceAuth); err != nil {
		return nil, err
	}
	return reader.ReadAuthSnapshot(ctx, stagedHome)
}

func (login *Login) RunStatus(ctx context.Context) (err error) {
	if login == nil || login.accounts == nil || login.codex == nil {
		return errors.New("login status is not configured")
	}
	runner, ok := login.codex.(selectedLoginRunner)
	if !ok {
		return errors.New("login status support is not configured")
	}
	stagedHome, err := login.accounts.CreateStagedHome()
	if err != nil {
		return err
	}
	defer func() {
		if cleanupErr := login.removeAll(stagedHome); cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
		}
	}()
	return runner.Run(ctx, stagedHome, []string{"login", "status"})
}

func (login *Login) RunSelectedStatus(ctx context.Context, home string) error {
	if login == nil || login.codex == nil {
		return errors.New("selected profile login is not configured")
	}
	runner, ok := login.codex.(selectedLoginRunner)
	if !ok {
		return errors.New("selected profile login status support is not configured")
	}
	return runner.Run(ctx, home, []string{"login", "status"})
}
