package routing

import (
	"sync"
	"sync/atomic"
)

type selectionState struct {
	sequence   atomic.Uint64
	notifierMu sync.RWMutex
	notifier   func()
}

// SetAdmissionSelectionNotifier connects routing-state changes to the outer
// admission wait. The callback runs after the routing mutex is released.
func (router *Router) SetAdmissionSelectionNotifier(notify func()) {
	if router == nil {
		return
	}
	router.selection.notifierMu.Lock()
	router.selection.notifier = notify
	router.selection.notifierMu.Unlock()
}

func (router *Router) notifySelectionChange() {
	if router == nil {
		return
	}
	router.selection.notifierMu.RLock()
	notify := router.selection.notifier
	router.selection.notifierMu.RUnlock()
	if notify != nil {
		notify()
	}
}
