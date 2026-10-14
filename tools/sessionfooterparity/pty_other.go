//go:build !linux

package main

import (
	"context"
	"errors"
)

type commandResult struct {
	output   []byte
	exitCode int
}

func runCommand(context.Context, string, []string, []string, bool) (commandResult, error) {
	return commandResult{}, errors.New("sessionfooterparity requires Linux PTY support")
}

func commandContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}
