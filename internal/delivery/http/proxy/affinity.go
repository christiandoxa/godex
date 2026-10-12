package proxy

import "context"

// ReleaseSessionAffinity clears routing state before native continuation.
func (proxy *Proxy) ReleaseSessionAffinity(ctx context.Context, sessionID string) error {
	if proxy == nil || proxy.router == nil {
		return nil
	}
	return proxy.router.ReleaseSessionAffinity(ctx, sessionID)
}
