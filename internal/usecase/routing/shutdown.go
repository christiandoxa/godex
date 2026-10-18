package routing

// StopAdmission prevents new execution permits and wakes queued requests.
// Active response bodies retain their existing ownership until closed.
func (router *Router) StopAdmission() {
	router.mu.Lock()
	defer router.mu.Unlock()
	if router.stopping {
		return
	}
	router.stopping = true
	if router.inflightChanged != nil {
		close(router.inflightChanged)
	}
	router.inflightChanged = make(chan struct{})
}
