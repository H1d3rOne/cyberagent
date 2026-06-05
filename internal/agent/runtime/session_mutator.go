package runtime

import (
	"context"
	"errors"
)

var (
	// ErrSessionMutatorNotAttached is returned when MutateSession is called
	// before a daemon/engine-native session mutator has been attached.
	ErrSessionMutatorNotAttached = errors.New("runtime: session mutator not attached")
)

// SessionMutationKind identifies one bounded session state mutation. The
// initial migration slice only moves legacy append-messages + optional planner
// apply behind this engine seam.
type SessionMutationKind string

const (
	SessionMutationAppendMessages    SessionMutationKind = "append_messages"
	SessionMutationRecordUserMessage SessionMutationKind = "record_user_message"
)

// SessionMutationRequest is the engine-owned boundary for one bounded session
// mutation. Turn carries stable runtime identifiers; Payload remains a
// migration adapter for daemon-private session/planner/message types until
// those types move runtime-side.
type SessionMutationRequest struct {
	Turn     TurnContext
	Kind     SessionMutationKind
	Payload  any
	Metadata map[string]any
}

// SessionMutationResult reports a bounded session mutation outcome. Payload is
// reserved for migration adapters that still return daemon-private snapshots.
type SessionMutationResult struct {
	Status  TurnResultStatus
	Reason  string
	Error   error
	Payload any
	Data    map[string]any
}

// SessionMutator executes one bounded session mutation.
type SessionMutator interface {
	MutateSession(ctx context.Context, req SessionMutationRequest) (SessionMutationResult, error)
}

// SessionMutatorFunc adapts a function into SessionMutator for tests and
// narrow migration adapters.
type SessionMutatorFunc func(context.Context, SessionMutationRequest) (SessionMutationResult, error)

func (f SessionMutatorFunc) MutateSession(ctx context.Context, req SessionMutationRequest) (SessionMutationResult, error) {
	if f == nil {
		return missingSessionMutatorResult(ErrSessionMutatorNotAttached), ErrSessionMutatorNotAttached
	}
	return f(ctx, req)
}

// AttachSessionMutator sets the mutator used by MutateSession. Passing nil is
// a supported detach operation and restores the explicit unsupported result.
func (e *Engine) AttachSessionMutator(mutator SessionMutator) {
	if e == nil {
		return
	}
	e.mu.Lock()
	e.sessionMutator = mutator
	e.mu.Unlock()
}

// SessionMutator returns the currently attached session mutator, if any.
func (e *Engine) SessionMutator() SessionMutator {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.sessionMutator
}

// MutateSession dispatches a SessionMutationRequest to the attached mutator. It
// does not emit lifecycle events by itself; BeginTurn/FinishTurn own persisted
// runtime lifecycle state.
func (e *Engine) MutateSession(ctx context.Context, req SessionMutationRequest) (SessionMutationResult, error) {
	if e == nil {
		return missingSessionMutatorResult(ErrNilEngine), ErrNilEngine
	}
	if ctx == nil {
		ctx = context.Background()
	}
	mutator := e.SessionMutator()
	if mutator == nil {
		return missingSessionMutatorResult(ErrSessionMutatorNotAttached), ErrSessionMutatorNotAttached
	}
	return mutator.MutateSession(ctx, req)
}

func missingSessionMutatorResult(err error) SessionMutationResult {
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	return SessionMutationResult{
		Status: TurnResultBlocked,
		Reason: reason,
		Error:  err,
	}
}
