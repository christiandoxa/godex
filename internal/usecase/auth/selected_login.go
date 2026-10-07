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

func (login *Login) RunSelected(ctx context.Context, deviceAuth bool) ([]byte, error) {
	arguments := []string{}
	if deviceAuth {
		arguments = append(arguments, "--device-auth")
	}
	return login.RunSelectedArguments(ctx, arguments)
}

func (login *Login) RunSelectedArguments(ctx context.Context, arguments []string) (authJSON []byte, err error) {
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
	if runner, ok := login.codex.(loginCodexArguments); ok {
		if _, err := runner.LoginArguments(ctx, stagedHome, append([]string(nil), arguments...)); err != nil {
			return nil, err
		}
	} else if len(arguments) == 0 {
		if _, err := login.codex.Login(ctx, stagedHome, false); err != nil {
			return nil, err
		}
	} else if len(arguments) == 1 && arguments[0] == "--device-auth" {
		if _, err := login.codex.Login(ctx, stagedHome, true); err != nil {
			return nil, err
		}
	} else {
		return nil, errors.New("Codex login passthrough support is not configured")
	}
	return reader.ReadAuthSnapshot(ctx, stagedHome)
}

func (login *Login) RunStatus(ctx context.Context) error {
	return login.RunStatusArguments(ctx, []string{"status"})
}

func (login *Login) RunStatusArguments(ctx context.Context, arguments []string) (err error) {
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
	return runner.Run(ctx, stagedHome, append([]string{"login"}, arguments...))
}

func (login *Login) RunSelectedStatus(ctx context.Context, home string) error {
	return login.RunSelectedStatusArguments(ctx, home, []string{"status"})
}

func (login *Login) RunSelectedStatusArguments(ctx context.Context, home string, arguments []string) error {
	if login == nil || login.codex == nil {
		return errors.New("selected profile login is not configured")
	}
	runner, ok := login.codex.(selectedLoginRunner)
	if !ok {
		return errors.New("selected profile login status support is not configured")
	}
	return runner.Run(ctx, home, append([]string{"login"}, arguments...))
}
