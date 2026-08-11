package interview

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"
)

type sqliteScanner interface {
	Scan(...any) error
}

const commandColumns = `
	id, principal_id, session_id, action, idempotency_key, subject_id, request_hash,
	status, result_json, error_json, created_at_ms, updated_at_ms`

func (s *SQLiteStore) CreateOrGetCommand(ctx context.Context, spec CommandSpec) (Command, bool, error) {
	if err := validateCommandSpec(spec); err != nil {
		return Command{}, false, err
	}
	now := time.Now().UTC().UnixMilli()
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO interview_commands (
			id, principal_id, session_id, action, idempotency_key, subject_id, request_hash,
			status, created_at_ms, updated_at_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING
	`, spec.ID, spec.PrincipalID, spec.SessionID, spec.Action, spec.IdempotencyKey, spec.SubjectID,
		spec.RequestHash, CommandPending, now, now)
	if err != nil {
		return Command{}, false, fmt.Errorf("interview persistence: create command: %w", err)
	}
	created, err := result.RowsAffected()
	if err != nil {
		return Command{}, false, fmt.Errorf("interview persistence: inspect command create: %w", err)
	}
	if created == 1 {
		command, err := s.GetCommand(ctx, spec.ID)
		return command, true, err
	}

	command, err := s.getCommandByIdempotencyKey(
		ctx, spec.PrincipalID, spec.SessionID, spec.Action, spec.IdempotencyKey,
	)
	if err == nil {
		if sameCommandSpec(command, spec) {
			return command, false, nil
		}
		return command, false, fmt.Errorf("%w: session %q key %q", ErrIdempotencyConflict, spec.SessionID, spec.IdempotencyKey)
	}
	if !errors.Is(err, ErrPersistenceNotFound) {
		return Command{}, false, err
	}

	if spec.SubjectID != "" {
		command, err = s.getCommandBySubject(ctx, spec.PrincipalID, spec.SessionID, spec.Action, spec.SubjectID)
		if err == nil {
			return command, false, fmt.Errorf("%w: session %q subject %q", ErrAnswerAlreadyCommitted, spec.SessionID, spec.SubjectID)
		}
		if !errors.Is(err, ErrPersistenceNotFound) {
			return Command{}, false, err
		}
	}

	return Command{}, false, fmt.Errorf("%w: command id %q", ErrPersistenceConflict, spec.ID)
}

func (s *SQLiteStore) GetCommand(ctx context.Context, id string) (Command, error) {
	return scanSQLiteCommand(s.db.QueryRowContext(ctx, `SELECT `+commandColumns+`
		FROM interview_commands WHERE id = ?`, id))
}

func (s *SQLiteStore) getCommandByIdempotencyKey(
	ctx context.Context,
	principalID, sessionID, action, key string,
) (Command, error) {
	return scanSQLiteCommand(s.db.QueryRowContext(ctx, `SELECT `+commandColumns+`
		FROM interview_commands
		WHERE principal_id = ? AND session_id = ? AND action = ? AND idempotency_key = ?`,
		principalID, sessionID, action, key))
}

func (s *SQLiteStore) getCommandBySubject(
	ctx context.Context,
	principalID, sessionID, action, subjectID string,
) (Command, error) {
	return scanSQLiteCommand(s.db.QueryRowContext(ctx, `SELECT `+commandColumns+`
		FROM interview_commands
		WHERE principal_id = ? AND session_id = ? AND action = ? AND subject_id = ?`,
		principalID, sessionID, action, subjectID))
}

func (s *SQLiteStore) TransitionCommand(
	ctx context.Context,
	id string,
	expected CommandStatus,
	transition CommandTransition,
) (Command, error) {
	if id == "" {
		return Command{}, errors.New("interview persistence: command id is empty")
	}
	if !validCommandStatus(expected) || !validCommandStatus(transition.Status) {
		return Command{}, errors.New("interview persistence: invalid command status")
	}
	resultJSON, err := optionalJSON(transition.Result, "command result")
	if err != nil {
		return Command{}, err
	}
	errorJSON, err := optionalJSON(transition.Error, "command error")
	if err != nil {
		return Command{}, err
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE interview_commands
		SET status = ?, result_json = ?, error_json = ?, updated_at_ms = ?
		WHERE id = ? AND status = ?
	`, transition.Status, resultJSON, errorJSON, time.Now().UTC().UnixMilli(), id, expected)
	if err != nil {
		return Command{}, fmt.Errorf("interview persistence: transition command %q: %w", id, err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return Command{}, fmt.Errorf("interview persistence: inspect command transition %q: %w", id, err)
	}
	if updated != 1 {
		if _, getErr := s.GetCommand(ctx, id); errors.Is(getErr, ErrPersistenceNotFound) {
			return Command{}, getErr
		} else if getErr != nil {
			return Command{}, getErr
		}
		return Command{}, fmt.Errorf("%w: command %q status changed", ErrPersistenceConflict, id)
	}
	return s.GetCommand(ctx, id)
}

func (s *SQLiteStore) CommitAnswer(ctx context.Context, spec AnswerCommitSpec) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if spec.CommandID == "" {
		return errors.New("interview persistence: answer command id is empty")
	}
	if spec.Session.ID == "" || spec.Event.SessionID != spec.Session.ID || spec.Event.CommandID != spec.CommandID {
		return errors.New("interview persistence: answer commit scope is invalid")
	}
	if spec.ExpectedVersion == math.MaxInt64 || spec.Session.Version != spec.ExpectedVersion+1 {
		return ErrStoreConflict
	}
	resultJSON, err := requiredJSON(spec.Result, "answer command result")
	if err != nil {
		return err
	}
	eventJSON, err := requiredJSON(spec.Event.Payload, "answer committed event")
	if err != nil {
		return err
	}
	if spec.Event.EventID == "" || spec.Event.Type == "" {
		return errors.New("interview persistence: answer committed event is invalid")
	}
	snapshot, err := marshalSQLiteSession(spec.Session)
	if err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("interview persistence: begin answer commit: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	updated, err := tx.ExecContext(ctx, `
		UPDATE interview_sessions
		SET version = ?, snapshot_json = ?, updated_at_ms = ?
		WHERE id = ? AND version = ?
	`, spec.Session.Version, snapshot, now.UnixMilli(), spec.Session.ID, spec.ExpectedVersion)
	if err != nil {
		return fmt.Errorf("interview persistence: update answer session: %w", err)
	}
	count, err := updated.RowsAffected()
	if err != nil {
		return fmt.Errorf("interview persistence: inspect answer session update: %w", err)
	}
	if count != 1 {
		var exists int
		if queryErr := tx.QueryRowContext(ctx, `SELECT 1 FROM interview_sessions WHERE id = ?`, spec.Session.ID).Scan(&exists); errors.Is(queryErr, sql.ErrNoRows) {
			return ErrStoreNotFound
		} else if queryErr != nil {
			return fmt.Errorf("interview persistence: inspect answer session: %w", queryErr)
		}
		return ErrStoreConflict
	}

	updated, err = tx.ExecContext(ctx, `
		UPDATE interview_commands
		SET status = ?, result_json = ?, error_json = NULL, updated_at_ms = ?
		WHERE id = ? AND session_id = ? AND action = ? AND status = ?
	`, CommandSucceeded, resultJSON, now.UnixMilli(), spec.CommandID, spec.Session.ID, ActionAnswer, CommandRunning)
	if err != nil {
		return fmt.Errorf("interview persistence: complete answer command: %w", err)
	}
	count, err = updated.RowsAffected()
	if err != nil {
		return fmt.Errorf("interview persistence: inspect answer command completion: %w", err)
	}
	if count != 1 {
		var status CommandStatus
		if queryErr := tx.QueryRowContext(ctx, `SELECT status FROM interview_commands WHERE id = ?`, spec.CommandID).Scan(&status); errors.Is(queryErr, sql.ErrNoRows) {
			return ErrPersistenceNotFound
		} else if queryErr != nil {
			return fmt.Errorf("interview persistence: inspect answer command: %w", queryErr)
		}
		return fmt.Errorf("%w: answer command %q status is %q", ErrPersistenceConflict, spec.CommandID, status)
	}

	var sequence int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO interview_session_cursors (session_id, last_sequence)
		VALUES (?, 1)
		ON CONFLICT(session_id) DO UPDATE
		SET last_sequence = interview_session_cursors.last_sequence + 1
		RETURNING last_sequence
	`, spec.Session.ID).Scan(&sequence)
	if err != nil {
		return fmt.Errorf("interview persistence: allocate answer event sequence: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO interview_session_events (
			session_id, sequence, event_id, command_id, event_type, payload_json, created_at_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, spec.Session.ID, sequence, spec.Event.EventID, spec.CommandID, spec.Event.Type, eventJSON, now.UnixMilli())
	if err != nil {
		return fmt.Errorf("interview persistence: append answer committed event: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("interview persistence: commit answer: %w", err)
	}
	return nil
}

func scanSQLiteCommand(scanner sqliteScanner) (Command, error) {
	var (
		command              Command
		resultJSON, errJSON  []byte
		createdMS, updatedMS int64
	)
	err := scanner.Scan(
		&command.ID, &command.PrincipalID, &command.SessionID, &command.Action, &command.IdempotencyKey,
		&command.SubjectID, &command.RequestHash, &command.Status, &resultJSON,
		&errJSON, &createdMS, &updatedMS,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Command{}, ErrPersistenceNotFound
	}
	if err != nil {
		return Command{}, fmt.Errorf("interview persistence: scan command: %w", err)
	}
	command.Result = cloneRawJSON(resultJSON)
	command.Error = cloneRawJSON(errJSON)
	command.CreatedAt = sqliteTime(createdMS)
	command.UpdatedAt = sqliteTime(updatedMS)
	return command, nil
}

func validateCommandSpec(spec CommandSpec) error {
	switch {
	case spec.ID == "":
		return errors.New("interview persistence: command id is empty")
	case spec.SessionID == "":
		return errors.New("interview persistence: command session id is empty")
	case spec.Action == "":
		return errors.New("interview persistence: command action is empty")
	case spec.IdempotencyKey == "":
		return errors.New("interview persistence: command idempotency key is empty")
	case spec.RequestHash == "":
		return errors.New("interview persistence: command request hash is empty")
	default:
		return nil
	}
}

func sameCommandSpec(command Command, spec CommandSpec) bool {
	return command.PrincipalID == spec.PrincipalID && command.SessionID == spec.SessionID && command.Action == spec.Action &&
		command.IdempotencyKey == spec.IdempotencyKey && command.SubjectID == spec.SubjectID &&
		command.RequestHash == spec.RequestHash
}

func validCommandStatus(status CommandStatus) bool {
	switch status {
	case CommandPending, CommandRunning, CommandSucceeded, CommandFailed:
		return true
	default:
		return false
	}
}

const eventColumns = `
	event_id, session_id, sequence, command_id, event_type, payload_json, created_at_ms`

func (s *SQLiteStore) AppendSessionEvent(ctx context.Context, spec SessionEventSpec) (SessionEvent, bool, error) {
	payload, err := requiredJSON(spec.Payload, "session event payload")
	if err != nil {
		return SessionEvent{}, false, err
	}
	switch {
	case spec.EventID == "":
		return SessionEvent{}, false, errors.New("interview persistence: event id is empty")
	case spec.SessionID == "":
		return SessionEvent{}, false, errors.New("interview persistence: event session id is empty")
	case spec.Type == "":
		return SessionEvent{}, false, errors.New("interview persistence: event type is empty")
	}
	spec.Payload = payload
	if existing, getErr := s.getSessionEventByID(ctx, spec.EventID); getErr == nil {
		if sameEventSpec(existing, spec) {
			return existing, false, nil
		}
		return existing, false, fmt.Errorf("%w: event id %q", ErrPersistenceConflict, spec.EventID)
	} else if !errors.Is(getErr, ErrPersistenceNotFound) {
		return SessionEvent{}, false, getErr
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SessionEvent{}, false, fmt.Errorf("interview persistence: begin event append: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var sequence int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO interview_session_cursors (session_id, last_sequence)
		VALUES (?, 1)
		ON CONFLICT(session_id) DO UPDATE
		SET last_sequence = interview_session_cursors.last_sequence + 1
		RETURNING last_sequence
	`, spec.SessionID).Scan(&sequence)
	if err != nil {
		return SessionEvent{}, false, fmt.Errorf("interview persistence: allocate event sequence: %w", err)
	}
	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO interview_session_events (
			session_id, sequence, event_id, command_id, event_type, payload_json, created_at_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, spec.SessionID, sequence, spec.EventID, spec.CommandID, spec.Type, payload, now.UnixMilli())
	if err != nil {
		_ = tx.Rollback()
		if existing, getErr := s.getSessionEventByID(ctx, spec.EventID); getErr == nil {
			if sameEventSpec(existing, spec) {
				return existing, false, nil
			}
			return existing, false, fmt.Errorf("%w: event id %q", ErrPersistenceConflict, spec.EventID)
		}
		return SessionEvent{}, false, fmt.Errorf("interview persistence: append event: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return SessionEvent{}, false, fmt.Errorf("interview persistence: commit event append: %w", err)
	}
	return SessionEvent{
		EventID: spec.EventID, SessionID: spec.SessionID, Sequence: sequence,
		CommandID: spec.CommandID, Type: spec.Type, Payload: cloneRawJSON(payload), CreatedAt: sqliteTime(now.UnixMilli()),
	}, true, nil
}

func (s *SQLiteStore) ListSessionEvents(ctx context.Context, sessionID string, afterSequence int64, limit int) ([]SessionEvent, error) {
	if sessionID == "" {
		return nil, errors.New("interview persistence: event session id is empty")
	}
	limit = boundedLimit(limit, 100, 1000)
	rows, err := s.db.QueryContext(ctx, `SELECT `+eventColumns+`
		FROM interview_session_events
		WHERE session_id = ? AND sequence > ?
		ORDER BY sequence ASC LIMIT ?`, sessionID, afterSequence, limit)
	if err != nil {
		return nil, fmt.Errorf("interview persistence: list session events: %w", err)
	}
	defer rows.Close()
	events := make([]SessionEvent, 0)
	for rows.Next() {
		event, scanErr := scanSQLiteEvent(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		events = append(events, event)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("interview persistence: iterate session events: %w", err)
	}
	return events, nil
}

func (s *SQLiteStore) getSessionEventByID(ctx context.Context, eventID string) (SessionEvent, error) {
	return scanSQLiteEvent(s.db.QueryRowContext(ctx, `SELECT `+eventColumns+`
		FROM interview_session_events WHERE event_id = ?`, eventID))
}

func scanSQLiteEvent(scanner sqliteScanner) (SessionEvent, error) {
	var event SessionEvent
	var payload []byte
	var createdMS int64
	err := scanner.Scan(&event.EventID, &event.SessionID, &event.Sequence, &event.CommandID,
		&event.Type, &payload, &createdMS)
	if errors.Is(err, sql.ErrNoRows) {
		return SessionEvent{}, ErrPersistenceNotFound
	}
	if err != nil {
		return SessionEvent{}, fmt.Errorf("interview persistence: scan event: %w", err)
	}
	event.Payload = cloneRawJSON(payload)
	event.CreatedAt = sqliteTime(createdMS)
	return event, nil
}

func sameEventSpec(event SessionEvent, spec SessionEventSpec) bool {
	return event.EventID == spec.EventID && event.SessionID == spec.SessionID &&
		event.CommandID == spec.CommandID && event.Type == spec.Type && bytes.Equal(event.Payload, spec.Payload)
}

const invocationColumns = `
	id, run_id, command_id, session_id, agent, request_hash, status, attempt,
	result_json, error_json, created_at_ms, updated_at_ms`

func (s *SQLiteStore) CreateOrGetModelInvocation(ctx context.Context, spec ModelInvocationSpec) (ModelInvocation, bool, error) {
	if err := validateInvocationSpec(spec); err != nil {
		return ModelInvocation{}, false, err
	}
	now := time.Now().UTC().UnixMilli()
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO interview_model_invocations (
			id, run_id, command_id, session_id, agent, request_hash, status, attempt,
			created_at_ms, updated_at_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING
	`, spec.ID, spec.RunID, spec.CommandID, spec.SessionID, spec.Agent, spec.RequestHash,
		InvocationPending, spec.Attempt, now, now)
	if err != nil {
		return ModelInvocation{}, false, fmt.Errorf("interview persistence: create model invocation: %w", err)
	}
	created, err := result.RowsAffected()
	if err != nil {
		return ModelInvocation{}, false, fmt.Errorf("interview persistence: inspect model invocation create: %w", err)
	}
	if created == 1 {
		invocation, err := s.GetModelInvocation(ctx, spec.ID)
		return invocation, true, err
	}
	invocation, err := s.GetModelInvocation(ctx, spec.ID)
	if errors.Is(err, ErrPersistenceNotFound) {
		invocation, err = s.getModelInvocationByRequest(ctx, spec.RunID, spec.RequestHash, spec.Attempt)
	}
	if err != nil {
		return ModelInvocation{}, false, err
	}
	if invocation.RunID == spec.RunID && invocation.CommandID == spec.CommandID &&
		invocation.SessionID == spec.SessionID && invocation.Agent == spec.Agent &&
		invocation.RequestHash == spec.RequestHash && invocation.Attempt == spec.Attempt {
		return invocation, false, nil
	}
	return invocation, false, fmt.Errorf("%w: model invocation %q", ErrPersistenceConflict, spec.ID)
}

func (s *SQLiteStore) GetModelInvocation(ctx context.Context, id string) (ModelInvocation, error) {
	return scanSQLiteInvocation(s.db.QueryRowContext(ctx, `SELECT `+invocationColumns+`
		FROM interview_model_invocations WHERE id = ?`, id))
}

func (s *SQLiteStore) getModelInvocationByRequest(ctx context.Context, runID, requestHash string, attempt int) (ModelInvocation, error) {
	return scanSQLiteInvocation(s.db.QueryRowContext(ctx, `SELECT `+invocationColumns+`
		FROM interview_model_invocations WHERE run_id = ? AND request_hash = ? AND attempt = ?`,
		runID, requestHash, attempt))
}

func (s *SQLiteStore) TransitionModelInvocation(
	ctx context.Context,
	id string,
	expected InvocationStatus,
	transition InvocationTransition,
) (ModelInvocation, error) {
	if id == "" {
		return ModelInvocation{}, errors.New("interview persistence: model invocation id is empty")
	}
	if !validInvocationStatus(expected) || !validInvocationStatus(transition.Status) {
		return ModelInvocation{}, errors.New("interview persistence: invalid invocation status")
	}
	resultJSON, err := optionalJSON(transition.Result, "model invocation result")
	if err != nil {
		return ModelInvocation{}, err
	}
	errorJSON, err := optionalJSON(transition.Error, "model invocation error")
	if err != nil {
		return ModelInvocation{}, err
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE interview_model_invocations
		SET status = ?, result_json = ?, error_json = ?, updated_at_ms = ?
		WHERE id = ? AND status = ?
	`, transition.Status, resultJSON, errorJSON, time.Now().UTC().UnixMilli(), id, expected)
	if err != nil {
		return ModelInvocation{}, fmt.Errorf("interview persistence: transition model invocation %q: %w", id, err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return ModelInvocation{}, fmt.Errorf("interview persistence: inspect model invocation transition %q: %w", id, err)
	}
	if updated != 1 {
		if _, getErr := s.GetModelInvocation(ctx, id); errors.Is(getErr, ErrPersistenceNotFound) {
			return ModelInvocation{}, getErr
		} else if getErr != nil {
			return ModelInvocation{}, getErr
		}
		return ModelInvocation{}, fmt.Errorf("%w: model invocation %q status changed", ErrPersistenceConflict, id)
	}
	return s.GetModelInvocation(ctx, id)
}

func (s *SQLiteStore) ListRecoverableModelInvocations(ctx context.Context, limit int) ([]ModelInvocation, error) {
	limit = boundedLimit(limit, 100, 1000)
	rows, err := s.db.QueryContext(ctx, `SELECT `+invocationColumns+`
		FROM interview_model_invocations
		WHERE status IN (?, ?)
		ORDER BY updated_at_ms ASC, id ASC LIMIT ?`, InvocationPending, InvocationRunning, limit)
	if err != nil {
		return nil, fmt.Errorf("interview persistence: list recoverable model invocations: %w", err)
	}
	defer rows.Close()
	invocations := make([]ModelInvocation, 0)
	for rows.Next() {
		invocation, scanErr := scanSQLiteInvocation(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		invocations = append(invocations, invocation)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("interview persistence: iterate recoverable model invocations: %w", err)
	}
	return invocations, nil
}

func scanSQLiteInvocation(scanner sqliteScanner) (ModelInvocation, error) {
	var invocation ModelInvocation
	var resultJSON, errorJSON []byte
	var createdMS, updatedMS int64
	err := scanner.Scan(&invocation.ID, &invocation.RunID, &invocation.CommandID,
		&invocation.SessionID, &invocation.Agent, &invocation.RequestHash, &invocation.Status,
		&invocation.Attempt, &resultJSON, &errorJSON, &createdMS, &updatedMS)
	if errors.Is(err, sql.ErrNoRows) {
		return ModelInvocation{}, ErrPersistenceNotFound
	}
	if err != nil {
		return ModelInvocation{}, fmt.Errorf("interview persistence: scan model invocation: %w", err)
	}
	invocation.Result = cloneRawJSON(resultJSON)
	invocation.Error = cloneRawJSON(errorJSON)
	invocation.CreatedAt = sqliteTime(createdMS)
	invocation.UpdatedAt = sqliteTime(updatedMS)
	return invocation, nil
}

func validateInvocationSpec(spec ModelInvocationSpec) error {
	switch {
	case spec.ID == "":
		return errors.New("interview persistence: model invocation id is empty")
	case spec.RunID == "":
		return errors.New("interview persistence: model invocation run id is empty")
	case spec.SessionID == "":
		return errors.New("interview persistence: model invocation session id is empty")
	case spec.Agent == "":
		return errors.New("interview persistence: model invocation agent is empty")
	case spec.RequestHash == "":
		return errors.New("interview persistence: model invocation request hash is empty")
	case spec.Attempt <= 0:
		return errors.New("interview persistence: model invocation attempt must be positive")
	default:
		return nil
	}
}

func validInvocationStatus(status InvocationStatus) bool {
	switch status {
	case InvocationPending, InvocationRunning, InvocationSucceeded, InvocationFailed, InvocationManualReview:
		return true
	default:
		return false
	}
}

const checkpointColumns = `
	id, run_id, command_id, session_id, sequence, state_json, created_at_ms`

func (s *SQLiteStore) SaveCheckpoint(ctx context.Context, spec CheckpointSpec) (Checkpoint, bool, error) {
	state, err := requiredJSON(spec.State, "checkpoint state")
	if err != nil {
		return Checkpoint{}, false, err
	}
	switch {
	case spec.ID == "":
		return Checkpoint{}, false, errors.New("interview persistence: checkpoint id is empty")
	case spec.RunID == "":
		return Checkpoint{}, false, errors.New("interview persistence: checkpoint run id is empty")
	case spec.SessionID == "":
		return Checkpoint{}, false, errors.New("interview persistence: checkpoint session id is empty")
	case spec.Sequence < 0:
		return Checkpoint{}, false, errors.New("interview persistence: checkpoint sequence is negative")
	}
	spec.State = state
	now := time.Now().UTC().UnixMilli()
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO interview_checkpoints (
			id, run_id, command_id, session_id, sequence, state_json, created_at_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING
	`, spec.ID, spec.RunID, spec.CommandID, spec.SessionID, spec.Sequence, state, now)
	if err != nil {
		return Checkpoint{}, false, fmt.Errorf("interview persistence: save checkpoint: %w", err)
	}
	created, err := result.RowsAffected()
	if err != nil {
		return Checkpoint{}, false, fmt.Errorf("interview persistence: inspect checkpoint save: %w", err)
	}
	if created == 1 {
		checkpoint, err := s.getCheckpointByID(ctx, spec.ID)
		return checkpoint, true, err
	}
	checkpoint, err := s.getCheckpointByID(ctx, spec.ID)
	if errors.Is(err, ErrPersistenceNotFound) {
		checkpoint, err = s.getCheckpointByRunSequence(ctx, spec.RunID, spec.Sequence)
	}
	if err != nil {
		return Checkpoint{}, false, err
	}
	if checkpoint.RunID == spec.RunID && checkpoint.CommandID == spec.CommandID &&
		checkpoint.SessionID == spec.SessionID && checkpoint.Sequence == spec.Sequence &&
		bytes.Equal(checkpoint.State, spec.State) {
		return checkpoint, false, nil
	}
	return checkpoint, false, fmt.Errorf("%w: checkpoint %q", ErrPersistenceConflict, spec.ID)
}

func (s *SQLiteStore) LoadLatestCheckpoint(ctx context.Context, runID string) (Checkpoint, error) {
	return scanSQLiteCheckpoint(s.db.QueryRowContext(ctx, `SELECT `+checkpointColumns+`
		FROM interview_checkpoints WHERE run_id = ? ORDER BY sequence DESC LIMIT 1`, runID))
}

func (s *SQLiteStore) getCheckpointByID(ctx context.Context, id string) (Checkpoint, error) {
	return scanSQLiteCheckpoint(s.db.QueryRowContext(ctx, `SELECT `+checkpointColumns+`
		FROM interview_checkpoints WHERE id = ?`, id))
}

func (s *SQLiteStore) getCheckpointByRunSequence(ctx context.Context, runID string, sequence int64) (Checkpoint, error) {
	return scanSQLiteCheckpoint(s.db.QueryRowContext(ctx, `SELECT `+checkpointColumns+`
		FROM interview_checkpoints WHERE run_id = ? AND sequence = ?`, runID, sequence))
}

func scanSQLiteCheckpoint(scanner sqliteScanner) (Checkpoint, error) {
	var checkpoint Checkpoint
	var state []byte
	var createdMS int64
	err := scanner.Scan(&checkpoint.ID, &checkpoint.RunID, &checkpoint.CommandID,
		&checkpoint.SessionID, &checkpoint.Sequence, &state, &createdMS)
	if errors.Is(err, sql.ErrNoRows) {
		return Checkpoint{}, ErrPersistenceNotFound
	}
	if err != nil {
		return Checkpoint{}, fmt.Errorf("interview persistence: scan checkpoint: %w", err)
	}
	checkpoint.State = cloneRawJSON(state)
	checkpoint.CreatedAt = sqliteTime(createdMS)
	return checkpoint, nil
}

const leaseColumns = `run_id, owner_id, lease_token, expires_at_ms, updated_at_ms`

func (s *SQLiteStore) AcquireRunLease(
	ctx context.Context,
	runID, ownerID, leaseToken string,
	now time.Time,
	ttl time.Duration,
) (RunLease, bool, error) {
	if err := validateLeaseInput(runID, ownerID, leaseToken, ttl); err != nil {
		return RunLease{}, false, err
	}
	now = now.UTC()
	expiresMS := now.Add(ttl).UnixMilli()
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO interview_run_leases (run_id, owner_id, lease_token, expires_at_ms, updated_at_ms)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(run_id) DO UPDATE SET
			owner_id = excluded.owner_id,
			lease_token = excluded.lease_token,
			expires_at_ms = excluded.expires_at_ms,
			updated_at_ms = excluded.updated_at_ms
		WHERE interview_run_leases.expires_at_ms <= ?
			OR (interview_run_leases.owner_id = excluded.owner_id
				AND interview_run_leases.lease_token = excluded.lease_token)
	`, runID, ownerID, leaseToken, expiresMS, now.UnixMilli(), now.UnixMilli())
	if err != nil {
		return RunLease{}, false, fmt.Errorf("interview persistence: acquire run lease: %w", err)
	}
	acquired, err := result.RowsAffected()
	if err != nil {
		return RunLease{}, false, fmt.Errorf("interview persistence: inspect run lease acquire: %w", err)
	}
	lease, err := s.GetRunLease(ctx, runID)
	if err != nil {
		return RunLease{}, false, err
	}
	if acquired != 1 {
		return lease, false, fmt.Errorf("%w: run %q owned by %q", ErrLeaseHeld, runID, lease.OwnerID)
	}
	return lease, true, nil
}

func (s *SQLiteStore) RenewRunLease(
	ctx context.Context,
	runID, ownerID, leaseToken string,
	now time.Time,
	ttl time.Duration,
) (RunLease, error) {
	if err := validateLeaseInput(runID, ownerID, leaseToken, ttl); err != nil {
		return RunLease{}, err
	}
	now = now.UTC()
	result, err := s.db.ExecContext(ctx, `
		UPDATE interview_run_leases
		SET expires_at_ms = ?, updated_at_ms = ?
		WHERE run_id = ? AND owner_id = ? AND lease_token = ? AND expires_at_ms > ?
	`, now.Add(ttl).UnixMilli(), now.UnixMilli(), runID, ownerID, leaseToken, now.UnixMilli())
	if err != nil {
		return RunLease{}, fmt.Errorf("interview persistence: renew run lease: %w", err)
	}
	renewed, err := result.RowsAffected()
	if err != nil {
		return RunLease{}, fmt.Errorf("interview persistence: inspect run lease renewal: %w", err)
	}
	if renewed != 1 {
		return RunLease{}, fmt.Errorf("%w: run %q", ErrLeaseLost, runID)
	}
	return s.GetRunLease(ctx, runID)
}

func (s *SQLiteStore) ReleaseRunLease(ctx context.Context, runID, ownerID, leaseToken string) error {
	if runID == "" || ownerID == "" || leaseToken == "" {
		return errors.New("interview persistence: run lease identity is incomplete")
	}
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM interview_run_leases WHERE run_id = ? AND owner_id = ? AND lease_token = ?
	`, runID, ownerID, leaseToken)
	if err != nil {
		return fmt.Errorf("interview persistence: release run lease: %w", err)
	}
	released, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("interview persistence: inspect run lease release: %w", err)
	}
	if released != 1 {
		return fmt.Errorf("%w: run %q", ErrLeaseLost, runID)
	}
	return nil
}

func (s *SQLiteStore) GetRunLease(ctx context.Context, runID string) (RunLease, error) {
	var lease RunLease
	var expiresMS, updatedMS int64
	err := s.db.QueryRowContext(ctx, `SELECT `+leaseColumns+`
		FROM interview_run_leases WHERE run_id = ?`, runID).Scan(
		&lease.RunID, &lease.OwnerID, &lease.LeaseToken, &expiresMS, &updatedMS,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return RunLease{}, ErrPersistenceNotFound
	}
	if err != nil {
		return RunLease{}, fmt.Errorf("interview persistence: scan run lease: %w", err)
	}
	lease.ExpiresAt = sqliteTime(expiresMS)
	lease.UpdatedAt = sqliteTime(updatedMS)
	return lease, nil
}

func validateLeaseInput(runID, ownerID, leaseToken string, ttl time.Duration) error {
	switch {
	case runID == "":
		return errors.New("interview persistence: run lease run id is empty")
	case ownerID == "":
		return errors.New("interview persistence: run lease owner id is empty")
	case leaseToken == "":
		return errors.New("interview persistence: run lease token is empty")
	case ttl <= 0:
		return errors.New("interview persistence: run lease ttl must be positive")
	default:
		return nil
	}
}

const outboxColumns = `
	id, session_id, sequence, event_type, payload_json, status, available_at_ms,
	claim_owner, claim_until_ms, attempts, last_error, created_at_ms, published_at_ms`

func (s *SQLiteStore) EnqueueOutbox(ctx context.Context, spec OutboxMessageSpec) (OutboxMessage, bool, error) {
	payload, err := requiredJSON(spec.Payload, "outbox payload")
	if err != nil {
		return OutboxMessage{}, false, err
	}
	switch {
	case spec.ID == "":
		return OutboxMessage{}, false, errors.New("interview persistence: outbox id is empty")
	case spec.SessionID == "":
		return OutboxMessage{}, false, errors.New("interview persistence: outbox session id is empty")
	case spec.Sequence <= 0:
		return OutboxMessage{}, false, errors.New("interview persistence: outbox sequence must be positive")
	case spec.Type == "":
		return OutboxMessage{}, false, errors.New("interview persistence: outbox type is empty")
	}
	spec.Payload = payload
	now := time.Now().UTC()
	if spec.AvailableAt.IsZero() {
		spec.AvailableAt = now
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO interview_outbox (
			id, session_id, sequence, event_type, payload_json, status,
			available_at_ms, created_at_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING
	`, spec.ID, spec.SessionID, spec.Sequence, spec.Type, payload, OutboxPending,
		spec.AvailableAt.UTC().UnixMilli(), now.UnixMilli())
	if err != nil {
		return OutboxMessage{}, false, fmt.Errorf("interview persistence: enqueue outbox message: %w", err)
	}
	created, err := result.RowsAffected()
	if err != nil {
		return OutboxMessage{}, false, fmt.Errorf("interview persistence: inspect outbox enqueue: %w", err)
	}
	if created == 1 {
		message, err := s.GetOutboxMessage(ctx, spec.ID)
		return message, true, err
	}
	message, err := s.GetOutboxMessage(ctx, spec.ID)
	if errors.Is(err, ErrPersistenceNotFound) {
		message, err = s.getOutboxByEvent(ctx, spec.SessionID, spec.Sequence, spec.Type)
	}
	if err != nil {
		return OutboxMessage{}, false, err
	}
	if message.SessionID == spec.SessionID && message.Sequence == spec.Sequence &&
		message.Type == spec.Type && bytes.Equal(message.Payload, spec.Payload) {
		return message, false, nil
	}
	return message, false, fmt.Errorf("%w: outbox message %q", ErrPersistenceConflict, spec.ID)
}

func (s *SQLiteStore) GetOutboxMessage(ctx context.Context, id string) (OutboxMessage, error) {
	return scanSQLiteOutbox(s.db.QueryRowContext(ctx, `SELECT `+outboxColumns+`
		FROM interview_outbox WHERE id = ?`, id))
}

func (s *SQLiteStore) getOutboxByEvent(ctx context.Context, sessionID string, sequence int64, eventType string) (OutboxMessage, error) {
	return scanSQLiteOutbox(s.db.QueryRowContext(ctx, `SELECT `+outboxColumns+`
		FROM interview_outbox WHERE session_id = ? AND sequence = ? AND event_type = ?`,
		sessionID, sequence, eventType))
}

func (s *SQLiteStore) ClaimOutbox(
	ctx context.Context,
	owner string,
	now time.Time,
	ttl time.Duration,
	limit int,
) ([]OutboxMessage, error) {
	if owner == "" {
		return nil, errors.New("interview persistence: outbox claim owner is empty")
	}
	if ttl <= 0 {
		return nil, errors.New("interview persistence: outbox claim ttl must be positive")
	}
	limit = boundedLimit(limit, 100, 1000)
	now = now.UTC()
	rows, err := s.db.QueryContext(ctx, `
		SELECT id FROM interview_outbox
		WHERE (status = ? AND available_at_ms <= ?)
			OR (status = ? AND claim_until_ms <= ?)
		ORDER BY available_at_ms ASC, created_at_ms ASC, id ASC
		LIMIT ?
	`, OutboxPending, now.UnixMilli(), OutboxClaimed, now.UnixMilli(), limit*4)
	if err != nil {
		return nil, fmt.Errorf("interview persistence: select outbox claims: %w", err)
	}
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("interview persistence: scan outbox claim candidate: %w", err)
		}
		ids = append(ids, id)
	}
	if err = rows.Close(); err != nil {
		return nil, fmt.Errorf("interview persistence: close outbox claim candidates: %w", err)
	}

	claimed := make([]OutboxMessage, 0, min(limit, len(ids)))
	for _, id := range ids {
		if len(claimed) == limit {
			break
		}
		result, updateErr := s.db.ExecContext(ctx, `
			UPDATE interview_outbox
			SET status = ?, claim_owner = ?, claim_until_ms = ?, attempts = attempts + 1, last_error = ''
			WHERE id = ? AND (
				(status = ? AND available_at_ms <= ?)
				OR (status = ? AND claim_until_ms <= ?)
			)
		`, OutboxClaimed, owner, now.Add(ttl).UnixMilli(), id,
			OutboxPending, now.UnixMilli(), OutboxClaimed, now.UnixMilli())
		if updateErr != nil {
			return nil, fmt.Errorf("interview persistence: claim outbox message %q: %w", id, updateErr)
		}
		updated, updateErr := result.RowsAffected()
		if updateErr != nil {
			return nil, fmt.Errorf("interview persistence: inspect outbox claim %q: %w", id, updateErr)
		}
		if updated != 1 {
			continue
		}
		message, getErr := s.GetOutboxMessage(ctx, id)
		if getErr != nil {
			return nil, getErr
		}
		claimed = append(claimed, message)
	}
	return claimed, nil
}

func (s *SQLiteStore) MarkOutboxPublished(ctx context.Context, id, owner string, publishedAt time.Time) error {
	if id == "" || owner == "" {
		return errors.New("interview persistence: outbox publish identity is incomplete")
	}
	if publishedAt.IsZero() {
		publishedAt = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE interview_outbox
		SET status = ?, published_at_ms = ?, claim_owner = '', claim_until_ms = NULL, last_error = ''
		WHERE id = ? AND status = ? AND claim_owner = ?
	`, OutboxPublished, publishedAt.UTC().UnixMilli(), id, OutboxClaimed, owner)
	if err != nil {
		return fmt.Errorf("interview persistence: publish outbox message %q: %w", id, err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("interview persistence: inspect outbox publish %q: %w", id, err)
	}
	if updated != 1 {
		return fmt.Errorf("%w: message %q", ErrOutboxClaimLost, id)
	}
	return nil
}

func (s *SQLiteStore) MarkOutboxFailed(
	ctx context.Context,
	id, owner, failure string,
	retryAt time.Time,
) error {
	if id == "" || owner == "" {
		return errors.New("interview persistence: outbox failure identity is incomplete")
	}
	if retryAt.IsZero() {
		retryAt = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE interview_outbox
		SET status = ?, available_at_ms = ?, claim_owner = '', claim_until_ms = NULL, last_error = ?
		WHERE id = ? AND status = ? AND claim_owner = ?
	`, OutboxPending, retryAt.UTC().UnixMilli(), failure, id, OutboxClaimed, owner)
	if err != nil {
		return fmt.Errorf("interview persistence: fail outbox message %q: %w", id, err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("interview persistence: inspect outbox failure %q: %w", id, err)
	}
	if updated != 1 {
		return fmt.Errorf("%w: message %q", ErrOutboxClaimLost, id)
	}
	return nil
}

func scanSQLiteOutbox(scanner sqliteScanner) (OutboxMessage, error) {
	var message OutboxMessage
	var payload []byte
	var availableMS, createdMS int64
	var claimUntilMS, publishedMS sql.NullInt64
	err := scanner.Scan(
		&message.ID, &message.SessionID, &message.Sequence, &message.Type, &payload,
		&message.Status, &availableMS, &message.ClaimOwner, &claimUntilMS,
		&message.Attempts, &message.LastError, &createdMS, &publishedMS,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return OutboxMessage{}, ErrPersistenceNotFound
	}
	if err != nil {
		return OutboxMessage{}, fmt.Errorf("interview persistence: scan outbox message: %w", err)
	}
	message.Payload = cloneRawJSON(payload)
	message.AvailableAt = sqliteTime(availableMS)
	message.CreatedAt = sqliteTime(createdMS)
	if claimUntilMS.Valid {
		message.ClaimUntil = sqliteTime(claimUntilMS.Int64)
	}
	if publishedMS.Valid {
		message.PublishedAt = sqliteTime(publishedMS.Int64)
	}
	return message, nil
}

func requiredJSON(raw json.RawMessage, field string) ([]byte, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	return compactJSON(raw, field)
}

func optionalJSON(raw json.RawMessage, field string) ([]byte, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	return compactJSON(raw, field)
}

func compactJSON(raw json.RawMessage, field string) ([]byte, error) {
	if !json.Valid(raw) {
		return nil, fmt.Errorf("interview persistence: %s is invalid JSON", field)
	}
	var compacted bytes.Buffer
	if err := json.Compact(&compacted, raw); err != nil {
		return nil, fmt.Errorf("interview persistence: compact %s: %w", field, err)
	}
	return compacted.Bytes(), nil
}

func cloneRawJSON(raw []byte) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func sqliteTime(milliseconds int64) time.Time {
	return time.UnixMilli(milliseconds).UTC()
}

func boundedLimit(limit, fallback, maximum int) int {
	if limit <= 0 {
		return fallback
	}
	if limit > maximum {
		return maximum
	}
	return limit
}

var _ PersistenceRepository = (*SQLiteStore)(nil)
