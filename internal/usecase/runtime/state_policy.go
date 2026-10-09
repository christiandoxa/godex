package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const defaultRuntimeStateSaveDebounce = 150 * time.Millisecond

const (
	RuntimeStateSaveQueuePressureThreshold      = 8
	RuntimeContinuationJournalPressureThreshold = 8
)

type QueuePressureThresholds struct {
	StateSave           int
	ContinuationJournal int
}

func RuntimeQueuePressureActive(stats QueueStats, thresholds QueuePressureThresholds) bool {
	return stats.StatePending >= thresholds.StateSave ||
		stats.ContinuationJournalPending >= thresholds.ContinuationJournal
}

func RuntimeQueueEnqueueBacklog(pending int) int {
	if pending <= 1 {
		return 0
	}
	return pending - 1
}

var (
	ErrStateSaveQueueFull = errors.New("runtime state save queue is full")
	ErrStateSaveClosed    = errors.New("runtime state save scheduler is closed")
)

// StateSaver is the narrow persistence port consumed by the scheduler. The
// section mask is state=0x03 (core=0x01, full=0x02), continuations=0x04,
// profile scores=0x08, usage snapshots=0x10, and backoffs=0x20.
type StateSaver interface {
	SaveSelected(context.Context, []byte, []byte, []byte, []byte, []byte, uint8) error
	SaveContinuationJournal(context.Context, []byte, int64) error
}

type RuntimeStateSaveStateSection uint8

const (
	RuntimeStateSaveStateNone RuntimeStateSaveStateSection = iota
	RuntimeStateSaveStateCore
	RuntimeStateSaveStateFull
)

// RuntimeStateSaveSections identifies the state parts changed by one mutation.
type RuntimeStateSaveSections struct {
	State          RuntimeStateSaveStateSection
	Continuations  bool
	ProfileScores  bool
	UsageSnapshots bool
	Backoffs       bool
}

func (sections RuntimeStateSaveSections) Union(other RuntimeStateSaveSections) RuntimeStateSaveSections {
	state := sections.State
	if other.State > state {
		state = other.State
	}
	return RuntimeStateSaveSections{
		State:          state,
		Continuations:  sections.Continuations || other.Continuations,
		ProfileScores:  sections.ProfileScores || other.ProfileScores,
		UsageSnapshots: sections.UsageSnapshots || other.UsageSnapshots,
		Backoffs:       sections.Backoffs || other.Backoffs,
	}
}

func (RuntimeStateSaveSections) Full() RuntimeStateSaveSections {
	return RuntimeStateSaveSections{
		State: RuntimeStateSaveStateFull, Continuations: true,
		ProfileScores: true, UsageSnapshots: true, Backoffs: true,
	}
}

// RuntimeStateMutation names the state transition that caused a save.
type RuntimeStateMutation struct {
	Kind  string
	Value string
}

const (
	MutationFullState                    = "full_state"
	MutationStartupAudit                 = "startup_audit"
	MutationStartupContinuationMigration = "startup_continuation_migration"
	MutationStartupBackoffSoften         = "startup_backoff_soften"
	MutationResponseIDs                  = "response_ids"
	MutationPreviousResponseOwner        = "previous_response_owner"
	MutationPreviousResponseNegative     = "previous_response_negative_cache"
	MutationPreviousResponseRelease      = "previous_response_release"
	MutationResponseTouch                = "response_touch"
	MutationTurnState                    = "turn_state"
	MutationTurnStateTouch               = "turn_state_touch"
	MutationSessionID                    = "session_id"
	MutationSessionTouch                 = "session_touch"
	MutationSessionAffinityRelease       = "session_affinity_release"
	MutationCompactLineage               = "compact_lineage"
	MutationCompactLineageRelease        = "compact_lineage_release"
	MutationCompactSessionTouch          = "compact_session_touch"
	MutationCompactTurnStateTouch        = "compact_turn_state_touch"
	MutationDeadResponseBindingClear     = "dead_response_binding_clear"
	MutationQuotaRelease                 = "quota_release"
	MutationAuthFailedRelease            = "auth_failed_release"
	MutationContinuationStale            = "continuation_stale"
	MutationProfileCommit                = "profile_commit"
	MutationUsageSnapshot                = "usage_snapshot"
	MutationProfileRetryBackoff          = "profile_retry_backoff"
	MutationProfileTransportBackoff      = "profile_transport_backoff"
	MutationProfileCircuitHalfOpenProbe  = "profile_circuit_half_open_probe"
	MutationProfileHealth                = "profile_health"
	MutationProfileCircuitClear          = "profile_circuit_clear"
	MutationProfileBadPairing            = "profile_bad_pairing"
	MutationProfileAuthBackoff           = "profile_auth_backoff"
	MutationProfileAuthBackoffCleared    = "profile_auth_backoff_cleared"
)

func (mutation RuntimeStateMutation) Reason() string {
	switch mutation.Kind {
	case MutationFullState, MutationStartupAudit, MutationStartupContinuationMigration, MutationStartupBackoffSoften:
		return mutation.Kind
	default:
		return mutation.Kind + ":" + mutation.Value
	}
}

// RuntimeStateSaveSchedulePlan is the deterministic policy for one mutation.
type RuntimeStateSaveSchedulePlan struct {
	Sections                    RuntimeStateSaveSections
	Debounce                    time.Duration
	RequiresContinuationJournal bool
}

func RuntimeStateSaveSchedule(mutation RuntimeStateMutation, debounce time.Duration) (RuntimeStateSaveSchedulePlan, error) {
	sections, journal, hot, ok := mutationPolicy(mutation.Kind)
	if !ok {
		return RuntimeStateSaveSchedulePlan{}, fmt.Errorf("unknown runtime state mutation %q", mutation.Kind)
	}
	if !hot {
		debounce = 0
	}
	return RuntimeStateSaveSchedulePlan{
		Sections: sections, Debounce: debounce, RequiresContinuationJournal: journal,
	}, nil
}

func RuntimeStateSaveSectionsFor(mutation RuntimeStateMutation) RuntimeStateSaveSections {
	sections, _, _, _ := mutationPolicy(mutation.Kind)
	return sections
}

func RuntimeStateSaveRequiresJournal(mutation RuntimeStateMutation) bool {
	_, journal, _, _ := mutationPolicy(mutation.Kind)
	return journal
}

func RuntimeStateSaveDebounce(mutation RuntimeStateMutation, debounce time.Duration) time.Duration {
	_, _, hot, _ := mutationPolicy(mutation.Kind)
	if hot {
		return debounce
	}
	return 0
}

func mutationPolicy(kind string) (RuntimeStateSaveSections, bool, bool, bool) {
	full := RuntimeStateSaveSections{State: RuntimeStateSaveStateFull, Continuations: true, ProfileScores: true, UsageSnapshots: true, Backoffs: true}
	core := RuntimeStateSaveSections{State: RuntimeStateSaveStateCore, Continuations: true}
	coreProfile := RuntimeStateSaveSections{State: RuntimeStateSaveStateCore, Continuations: true, ProfileScores: true}
	switch kind {
	case MutationFullState, MutationStartupAudit, MutationStartupContinuationMigration:
		return full, false, false, true
	case MutationStartupBackoffSoften, MutationProfileTransportBackoff, MutationProfileCircuitHalfOpenProbe:
		return RuntimeStateSaveSections{Backoffs: true}, false, false, true
	case MutationResponseIDs, MutationPreviousResponseOwner:
		return coreProfile, true, true, true
	case MutationPreviousResponseNegative:
		return coreProfile, false, false, true
	case MutationPreviousResponseRelease:
		return coreProfile, true, false, true
	case MutationResponseTouch, MutationTurnStateTouch, MutationSessionTouch,
		MutationCompactSessionTouch, MutationCompactTurnStateTouch:
		return core, false, true, true
	case MutationTurnState, MutationSessionID, MutationCompactLineage, MutationCompactLineageRelease:
		return core, true, true, true
	case MutationSessionAffinityRelease, MutationDeadResponseBindingClear, MutationQuotaRelease:
		return core, true, false, true
	case MutationAuthFailedRelease:
		return full, true, false, true
	case MutationContinuationStale:
		return core, false, false, true
	case MutationProfileCommit:
		return RuntimeStateSaveSections{State: RuntimeStateSaveStateCore, ProfileScores: true, Backoffs: true}, false, false, true
	case MutationUsageSnapshot, MutationProfileRetryBackoff:
		return RuntimeStateSaveSections{UsageSnapshots: true, Backoffs: true}, false, false, true
	case MutationProfileHealth, MutationProfileCircuitClear:
		return RuntimeStateSaveSections{ProfileScores: true, Backoffs: true}, false, false, true
	case MutationProfileBadPairing, MutationProfileAuthBackoff, MutationProfileAuthBackoffCleared:
		return RuntimeStateSaveSections{ProfileScores: true}, false, false, true
	default:
		return RuntimeStateSaveSections{}, false, false, false
	}
}
