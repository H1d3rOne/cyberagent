package runtime

import (
	"context"
	"errors"
	"strings"
	"time"
)

// BeginTurn persists the task/run/turn state for one runtime turn and emits
// the same task_created, run_started and turn_started events previously
// produced by the daemon bridge. The daemon still adapts session state into
// TurnContext; this method owns only engine state and event publication.
func (e *Engine) BeginTurn(ctx context.Context, turn TurnContext) (TurnContext, error) {
	if e == nil {
		return TurnContext{}, ErrNilEngine
	}
	if ctx == nil {
		ctx = context.Background()
	}
	turn = normalizeTurnContext(turn)
	var errs []error
	if _, err := e.Submit(ctx, Task{
		ID:             turn.TaskID,
		SessionID:      turn.SessionID,
		Origin:         turn.Origin,
		Goal:           trimTurnLifecycleSummary(firstNonEmptyString(turn.Goal, turn.Prompt), 360),
		Prompt:         trimTurnLifecycleSummary(turn.Prompt, 1000),
		OperationLevel: turn.OperationLevel,
		Created:        turn.Timestamp,
	}); err != nil {
		errs = append(errs, err)
	}
	if _, err := e.SubmitRun(ctx, Run{
		ID:        turn.RunID,
		TaskID:    turn.TaskID,
		State:     string(RunRunning),
		Turn:      turn.TurnIndex,
		CreatedAt: turn.Timestamp,
	}); err != nil {
		errs = append(errs, err)
	}
	if err := e.saveTurn(ctx, Turn{ID: turn.TurnID, RunID: turn.RunID, Index: turn.TurnIndex, State: string(TurnRunning)}); err != nil {
		errs = append(errs, err)
	}
	data := map[string]any{
		"session_id": turn.SessionID,
		"message_id": turn.MessageID,
		"origin":     turn.Origin,
		"prompt":     turn.Prompt,
	}
	e.Publish(ctx, Event{
		Type:      EventTurnStarted,
		TaskID:    turn.TaskID,
		RunID:     turn.RunID,
		TurnID:    turn.TurnID,
		Data:      data,
		Timestamp: turn.Timestamp,
	})
	return turn, errors.Join(errs...)
}

// FinishTurn persists terminal turn state and emits the legacy turn_finished /
// turn_failed plus run terminal/approval events. A non-nil TurnResult.Error is
// the compatibility signal for turn_failed; a failed status without Error still
// emits turn_finished with status=failed, matching the daemon's previous
// response-derived completion behavior.
func (e *Engine) FinishTurn(ctx context.Context, turn TurnContext, result TurnResult) error {
	if e == nil {
		return ErrNilEngine
	}
	if ctx == nil {
		ctx = context.Background()
	}
	turn = normalizeTurnContext(turn)
	result = normalizeTurnResult(result)
	var errs []error
	turnState := string(TurnFinished)
	if result.Error != nil {
		turnState = string(TurnFailed)
	}
	if err := e.saveTurn(ctx, Turn{ID: turn.TurnID, RunID: turn.RunID, Index: turn.TurnIndex, State: turnState}); err != nil {
		errs = append(errs, err)
	}
	data := cloneEventData(result.Data)
	data["session_id"] = turn.SessionID
	data["status"] = string(result.Status)
	if strings.TrimSpace(result.Reason) != "" {
		data["reason"] = result.Reason
	}
	if result.Error != nil {
		data["error"] = result.Error.Error()
		e.Publish(ctx, Event{
			Type:      EventTurnFailed,
			TaskID:    turn.TaskID,
			RunID:     turn.RunID,
			TurnID:    turn.TurnID,
			Data:      data,
			Timestamp: time.Now().UTC(),
		})
		if err := e.FinishRun(ctx, turn.RunID, false); err != nil {
			errs = append(errs, err)
		}
		return errors.Join(errs...)
	}

	e.Publish(ctx, Event{
		Type:      EventTurnFinished,
		TaskID:    turn.TaskID,
		RunID:     turn.RunID,
		TurnID:    turn.TurnID,
		Data:      data,
		Timestamp: time.Now().UTC(),
	})
	switch result.Status {
	case TurnResultApprovalRequired:
		if err := e.RequestApproval(turn.RunID, result.Reason); err != nil {
			errs = append(errs, err)
		}
	case TurnResultFinished:
		if err := e.FinishRun(ctx, turn.RunID, true); err != nil {
			errs = append(errs, err)
		}
	case TurnResultFailed:
		if err := e.FinishRun(ctx, turn.RunID, false); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (e *Engine) saveTurn(ctx context.Context, turn Turn) error {
	if e == nil {
		return ErrNilEngine
	}
	e.mu.Lock()
	e.turns[turn.ID] = &turn
	store := e.store
	e.mu.Unlock()
	if store != nil {
		return store.SaveTurn(ctx, turn)
	}
	return nil
}

func normalizeTurnContext(turn TurnContext) TurnContext {
	turn.SessionID = strings.TrimSpace(turn.SessionID)
	turn.TaskID = strings.TrimSpace(turn.TaskID)
	turn.RunID = strings.TrimSpace(turn.RunID)
	turn.TurnID = strings.TrimSpace(turn.TurnID)
	turn.MessageID = strings.TrimSpace(turn.MessageID)
	turn.Origin = firstNonEmptyString(turn.Origin, "user")
	turn.OperationLevel = strings.TrimSpace(turn.OperationLevel)
	if turn.TurnIndex <= 0 {
		turn.TurnIndex = 1
	}
	if turn.Timestamp.IsZero() {
		turn.Timestamp = time.Now().UTC()
	}
	return turn
}

func normalizeTurnResult(result TurnResult) TurnResult {
	if strings.TrimSpace(string(result.Status)) == "" {
		result.Status = TurnResultRunning
	}
	result.Reason = strings.TrimSpace(result.Reason)
	return result
}

func trimTurnLifecycleSummary(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max] + "…"
}
