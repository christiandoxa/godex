package routing

import (
	"sync"
	"sync/atomic"
)

type selectionState struct {
	sequence   atomic.Uint64
	notifierMu sync.RWMutex
	notifier   func()
	changed    chan struct{}
	revision   atomic.Uint64
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
	router.selection.notifierMu.Lock()
	if router.selection.changed == nil {
		router.selection.changed = make(chan struct{})
	} else {
		close(router.selection.changed)
		router.selection.changed = make(chan struct{})
	}
	router.selection.revision.Add(1)
	notify := router.selection.notifier
	router.selection.notifierMu.Unlock()
	if notify != nil {
		notify()
	}
}

func (router *Router) selectionWait() (uint64, <-chan struct{}) {
	if router == nil {
		return 0, nil
	}
	router.selection.notifierMu.RLock()
	revision, changed := router.selection.revision.Load(), router.selection.changed
	router.selection.notifierMu.RUnlock()
	return revision, changed
}
