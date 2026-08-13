package runtime

import (
	"context"

	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
)

type Proxy interface {
	Start() error
	Endpoint() string
	Close(context.Context) error
}

type ProxyFactory func(proxyconfig.Config) (Proxy, error)
