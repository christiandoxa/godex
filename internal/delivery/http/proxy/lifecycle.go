package proxy

type requestPhase uint8

const (
	requestUncommitted requestPhase = iota
	requestCommitted
	requestCompleted
	requestFailedAfterCommit
)

type requestLifecycle struct {
	phase requestPhase
}

func (lifecycle *requestLifecycle) canAttempt() bool {
	return lifecycle.phase == requestUncommitted
}

func (lifecycle *requestLifecycle) commit() {
	if lifecycle.phase == requestUncommitted {
		lifecycle.phase = requestCommitted
	}
}

func (lifecycle *requestLifecycle) complete() {
	if lifecycle.phase == requestCommitted {
		lifecycle.phase = requestCompleted
	}
}

func (lifecycle *requestLifecycle) failAfterCommit() {
	if lifecycle.phase == requestCommitted {
		lifecycle.phase = requestFailedAfterCommit
	}
}
