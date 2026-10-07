package superexpose

import (
	"context"
	"io"
)

type openAITunnelStarter func(string, string) (*openAITunnelProcess, error)

func runExecServer(ctx context.Context, options Options, out, errOut io.Writer) error {
	return runExecServerWithTunnelStarter(ctx, options, out, errOut, startOpenAITunnel)
}
