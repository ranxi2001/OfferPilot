package interview

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"
)

func TestSQLitePersistenceMigratesV1SnapshotWithoutChangingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "interviews.sqlite")
	session := sqliteTestSession()
	payload, err := marshalSQLiteSession(session)
	if err != nil {
		t.Fatalf("marshalSQLiteSession() error = %v", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	for _, statement := range []string{
		`CREATE TABLE interview_store_migrations (
			version INTEGER PRIMARY KEY,
			applied_at_ms INTEGER NOT NULL
		)`,
		`CREATE TABLE interview_sessions (
			id TEXT PRIMARY KEY,
			version INTEGER NOT NULL CHECK (version >= 0),
			snapshot_json BLOB NOT NULL,
			created_at_ms INTEGER NOT NULL,
			updated_at_ms INTEGER NOT NULL
		)`,
	} {
		if _, err = db.Exec(statement); err != nil {
			_ = db.Close()
			t.Fatalf("create v1 schema error = %v", err)
		}
	}
	if _, err = db.Exec(`INSERT INTO interview_store_migrations (version, applied_at_ms) VALUES (1, ?)`, time.Now().UnixMilli()); err != nil {
		_ = db.Close()
		t.Fatalf("record v1 migration error = %v", err)
	}
	if _, err = db.Exec(`
		INSERT INTO interview_sessions (id, version, snapshot_json, created_at_ms, updated_at_ms)
		VALUES (?, ?, ?, ?, ?)
	`, session.ID, session.Version, payload, time.Now().UnixMilli(), time.Now().UnixMilli()); err != nil {
		_ = db.Close()
		t.Fatalf("insert v1 snapshot error = %v", err)
	}
	if err = db.Close(); err != nil {
		t.Fatalf("close v1 database error = %v", err)
	}

	store, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatalf("OpenSQLiteStore(v1) error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	loaded, err := store.Load(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("Load(v1 snapshot) error = %v", err)
	}
	if !reflect.DeepEqual(loaded, session) {
		t.Fatalf("Load(v1 snapshot) changed data: got %#v, want %#v", loaded, session)
	}

	loaded.Version++
	loaded.ClientSessionID = "after-v3-migration"
	if err = store.Save(context.Background(), loaded, session.Version); err != nil {
		t.Fatalf("Save(after migration) error = %v", err)
	}
	var migrationVersion int
	if err = store.db.QueryRow(`SELECT MAX(version) FROM interview_store_migrations`).Scan(&migrationVersion); err != nil {
		t.Fatalf("read migration version error = %v", err)
	}
	if migrationVersion != 3 {
		t.Fatalf("migration version = %d, want 3", migrationVersion)
	}
}

func TestSQLitePersistenceMigratesPublishedV2CommandScope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "interviews.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE interview_store_migrations (
		version INTEGER PRIMARY KEY,
		applied_at_ms INTEGER NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	for _, migration := range sqliteMigrations[:2] {
		for _, statement := range migration.statements {
			if _, err = db.Exec(statement); err != nil {
				t.Fatalf("apply published migration %d: %v", migration.version, err)
			}
		}
		if _, err = db.Exec(`INSERT INTO interview_store_migrations (version, applied_at_ms) VALUES (?, ?)`, migration.version, time.Now().UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(`INSERT INTO interview_commands (
		id, session_id, action, idempotency_key, subject_id, request_hash,
		status, created_at_ms, updated_at_ms
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"command-v2", "session-1", "answer", "client-answer-1", "question-1", "sha256:v2",
		CommandSucceeded, time.Now().UnixMilli(), time.Now().UnixMilli(),
	); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatalf("OpenSQLiteStore(v2) error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	command, err := store.GetCommand(context.Background(), "command-v2")
	if err != nil || command.PrincipalID != "" || command.RequestHash != "sha256:v2" || command.Status != CommandSucceeded {
		t.Fatalf("migrated command = %+v error %v", command, err)
	}
	created, wasCreated, err := store.CreateOrGetCommand(context.Background(), CommandSpec{
		ID: "command-v3", PrincipalID: "principal-2", SessionID: "session-1",
		Action: "start", IdempotencyKey: "client-answer-1", RequestHash: "sha256:v3",
	})
	if err != nil || !wasCreated || created.ID != "command-v3" {
		t.Fatalf("expanded v3 command scope = %+v created %v error %v", created, wasCreated, err)
	}
	var migrationVersion int
	if err = store.db.QueryRow(`SELECT MAX(version) FROM interview_store_migrations`).Scan(&migrationVersion); err != nil || migrationVersion != 3 {
		t.Fatalf("migration version = %d error %v, want 3", migrationVersion, err)
	}
}

func TestSQLitePersistenceCommandIdempotencyAndAnswerUniqueness(t *testing.T) {
	store := openTestSQLiteStore(t)
	ctx := context.Background()
	spec := CommandSpec{
		ID: "command-1", SessionID: "session-1", Action: "answer",
		IdempotencyKey: "key-1", SubjectID: "question-1", RequestHash: "sha256:answer-a",
	}
	first, created, err := store.CreateOrGetCommand(ctx, spec)
	if err != nil || !created {
		t.Fatalf("first CreateOrGetCommand() = created %v error %v", created, err)
	}

	retry := spec
	retry.ID = "command-retry"
	again, created, err := store.CreateOrGetCommand(ctx, retry)
	if err != nil || created {
		t.Fatalf("retry CreateOrGetCommand() = created %v error %v", created, err)
	}
	if again.ID != first.ID {
		t.Fatalf("retry command id = %q, want %q", again.ID, first.ID)
	}

	changedPayload := retry
	changedPayload.RequestHash = "sha256:answer-b"
	if _, _, err = store.CreateOrGetCommand(ctx, changedPayload); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("same key different hash error = %v, want ErrIdempotencyConflict", err)
	}

	changedKey := spec
	changedKey.ID = "command-other-key"
	changedKey.IdempotencyKey = "key-2"
	if _, _, err = store.CreateOrGetCommand(ctx, changedKey); !errors.Is(err, ErrAnswerAlreadyCommitted) {
		t.Fatalf("same answer different key error = %v, want ErrAnswerAlreadyCommitted", err)
	}

	changedAnswer := changedKey
	changedAnswer.ID = "command-other-answer"
	changedAnswer.IdempotencyKey = "key-3"
	changedAnswer.RequestHash = "sha256:answer-b"
	if _, _, err = store.CreateOrGetCommand(ctx, changedAnswer); !errors.Is(err, ErrAnswerAlreadyCommitted) {
		t.Fatalf("second answer for question error = %v, want ErrAnswerAlreadyCommitted", err)
	}

	if count := sqliteRowCount(t, store, "interview_commands"); count != 1 {
		t.Fatalf("command row count = %d, want 1", count)
	}
	succeeded, err := store.TransitionCommand(ctx, first.ID, CommandPending, CommandTransition{
		Status: CommandSucceeded,
		Result: json.RawMessage(`{"answerId":"answer-1"}`),
	})
	if err != nil || succeeded.Status != CommandSucceeded {
		t.Fatalf("TransitionCommand() = status %q error %v", succeeded.Status, err)
	}
	if _, err = store.TransitionCommand(ctx, first.ID, CommandPending, CommandTransition{Status: CommandFailed}); !errors.Is(err, ErrPersistenceConflict) {
		t.Fatalf("stale TransitionCommand() error = %v, want ErrPersistenceConflict", err)
	}
}

func TestSQLitePersistenceCommandIdempotencyScopeIncludesPrincipalAndAction(t *testing.T) {
	store := openTestSQLiteStore(t)
	ctx := context.Background()
	base := CommandSpec{
		ID: "command-base", PrincipalID: "principal-a", SessionID: "session-1",
		Action: "answer", IdempotencyKey: "client-command-1", RequestHash: "sha256:base",
	}
	first, created, err := store.CreateOrGetCommand(ctx, base)
	if err != nil || !created {
		t.Fatalf("base CreateOrGetCommand() = created %v error %v", created, err)
	}

	otherAction := base
	otherAction.ID = "command-start"
	otherAction.Action = "start"
	otherAction.RequestHash = "sha256:start"
	if command, actionCreated, actionErr := store.CreateOrGetCommand(ctx, otherAction); actionErr != nil || !actionCreated || command.ID == first.ID {
		t.Fatalf("other action CreateOrGetCommand() = id %q created %v error %v", command.ID, actionCreated, actionErr)
	}

	otherPrincipal := base
	otherPrincipal.ID = "command-principal-b"
	otherPrincipal.PrincipalID = "principal-b"
	otherPrincipal.RequestHash = "sha256:principal-b"
	if command, principalCreated, principalErr := store.CreateOrGetCommand(ctx, otherPrincipal); principalErr != nil || !principalCreated || command.ID == first.ID {
		t.Fatalf("other principal CreateOrGetCommand() = id %q created %v error %v", command.ID, principalCreated, principalErr)
	}

	if count := sqliteRowCount(t, store, "interview_commands"); count != 3 {
		t.Fatalf("command row count = %d, want 3", count)
	}
}

func TestSQLitePersistenceConcurrentCommandIdempotencyAcrossStores(t *testing.T) {
	left, right := openSharedSQLiteStores(t)
	ctx := context.Background()
	specs := []CommandSpec{
		{ID: "command-left", SessionID: "session-1", Action: "start", IdempotencyKey: "same-key", RequestHash: "sha256:start"},
		{ID: "command-right", SessionID: "session-1", Action: "start", IdempotencyKey: "same-key", RequestHash: "sha256:start"},
	}
	stores := []*SQLiteStore{left, right}

	start := make(chan struct{})
	results := make(chan struct {
		command Command
		created bool
		err     error
	}, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for index := range stores {
		index := index
		go func() {
			ready.Done()
			<-start
			command, created, err := stores[index].CreateOrGetCommand(ctx, specs[index])
			results <- struct {
				command Command
				created bool
				err     error
			}{command, created, err}
		}()
	}
	ready.Wait()
	close(start)

	var ids []string
	createdCount := 0
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatalf("concurrent CreateOrGetCommand() error = %v", result.err)
		}
		if result.created {
			createdCount++
		}
		ids = append(ids, result.command.ID)
	}
	if createdCount != 1 || ids[0] != ids[1] {
		t.Fatalf("concurrent commands = ids %v created count %d, want one shared command", ids, createdCount)
	}
	if count := sqliteRowCount(t, left, "interview_commands"); count != 1 {
		t.Fatalf("command row count = %d, want 1", count)
	}
}

func TestSQLitePersistenceConcurrentAnswerDeduplicationAcrossStores(t *testing.T) {
	left, right := openSharedSQLiteStores(t)
	ctx := context.Background()
	stores := []*SQLiteStore{left, right}
	specs := []CommandSpec{
		{ID: "answer-left", SessionID: "session-1", Action: "answer", IdempotencyKey: "left-key", SubjectID: "question-1", RequestHash: "sha256:answer"},
		{ID: "answer-right", SessionID: "session-1", Action: "answer", IdempotencyKey: "right-key", SubjectID: "question-1", RequestHash: "sha256:answer"},
	}
	start := make(chan struct{})
	results := make(chan struct {
		command Command
		created bool
		err     error
	}, 2)
	for index := range stores {
		index := index
		go func() {
			<-start
			command, created, err := stores[index].CreateOrGetCommand(ctx, specs[index])
			results <- struct {
				command Command
				created bool
				err     error
			}{command, created, err}
		}()
	}
	close(start)

	createdCount := 0
	conflictCount := 0
	for range 2 {
		result := <-results
		if errors.Is(result.err, ErrAnswerAlreadyCommitted) {
			conflictCount++
			continue
		}
		if result.err != nil {
			t.Fatalf("concurrent answer command error = %v", result.err)
		}
		if result.created {
			createdCount++
		}
	}
	if createdCount != 1 || conflictCount != 1 {
		t.Fatalf("answer commands = created %d conflicts %d, want one accepted answer and one conflict", createdCount, conflictCount)
	}
	if count := sqliteRowCount(t, left, "interview_commands"); count != 1 {
		t.Fatalf("answer command row count = %d, want 1", count)
	}
}

func TestSQLitePersistenceConcurrentEventSequencesAreMonotonic(t *testing.T) {
	left, right := openSharedSQLiteStores(t)
	stores := []*SQLiteStore{left, right}
	ctx := context.Background()
	const eventCount = 40
	start := make(chan struct{})
	results := make(chan error, eventCount)
	var ready sync.WaitGroup
	ready.Add(eventCount)
	for index := range eventCount {
		index := index
		go func() {
			ready.Done()
			<-start
			_, created, err := stores[index%len(stores)].AppendSessionEvent(ctx, SessionEventSpec{
				EventID:   fmt.Sprintf("event-%02d", index),
				SessionID: "session-1",
				Type:      "step.completed",
				Payload:   json.RawMessage(fmt.Sprintf(`{"index":%d}`, index)),
			})
			if err == nil && !created {
				err = errors.New("new event was not created")
			}
			results <- err
		}()
	}
	ready.Wait()
	close(start)
	for range eventCount {
		if err := <-results; err != nil {
			t.Fatalf("AppendSessionEvent() error = %v", err)
		}
	}

	events, err := left.ListSessionEvents(ctx, "session-1", 0, eventCount+1)
	if err != nil {
		t.Fatalf("ListSessionEvents() error = %v", err)
	}
	if len(events) != eventCount {
		t.Fatalf("event count = %d, want %d", len(events), eventCount)
	}
	sequences := make([]int, 0, len(events))
	for index, event := range events {
		if event.Sequence != int64(index+1) {
			t.Fatalf("event[%d] sequence = %d, want %d", index, event.Sequence, index+1)
		}
		sequences = append(sequences, int(event.Sequence))
	}
	sort.Ints(sequences)
	for index, sequence := range sequences {
		if sequence != index+1 {
			t.Fatalf("sorted sequence[%d] = %d, want %d", index, sequence, index+1)
		}
	}

	retrySpec := SessionEventSpec{
		EventID: "event-00", SessionID: "session-1", Type: "step.completed", Payload: json.RawMessage(`{"index":0}`),
	}
	retry, created, err := right.AppendSessionEvent(ctx, retrySpec)
	if err != nil || created {
		t.Fatalf("duplicate AppendSessionEvent() = created %v error %v", created, err)
	}
	if retry.Sequence < 1 || retry.Sequence > eventCount {
		t.Fatalf("duplicate event sequence = %d", retry.Sequence)
	}
}

func TestSQLitePersistenceRecordsSurviveReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "interviews.sqlite")
	ctx := context.Background()
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	first, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatalf("first OpenSQLiteStore() error = %v", err)
	}
	command, _, err := first.CreateOrGetCommand(ctx, CommandSpec{
		ID: "command-1", SessionID: "session-1", Action: "answer",
		IdempotencyKey: "key-1", SubjectID: "question-1", RequestHash: "sha256:answer",
	})
	if err != nil {
		t.Fatalf("CreateOrGetCommand() error = %v", err)
	}
	if _, err = first.TransitionCommand(ctx, command.ID, CommandPending, CommandTransition{Status: CommandRunning}); err != nil {
		t.Fatalf("TransitionCommand() error = %v", err)
	}
	if _, _, err = first.AppendSessionEvent(ctx, SessionEventSpec{
		EventID: "event-1", SessionID: "session-1", CommandID: command.ID,
		Type: "answer.accepted", Payload: json.RawMessage(`{"answerId":"answer-1"}`),
	}); err != nil {
		t.Fatalf("AppendSessionEvent() error = %v", err)
	}
	invocation, _, err := first.CreateOrGetModelInvocation(ctx, ModelInvocationSpec{
		ID: "invocation-1", RunID: "run-1", CommandID: command.ID, SessionID: "session-1",
		Agent: "assessor", RequestHash: "sha256:prompt", Attempt: 1,
	})
	if err != nil {
		t.Fatalf("CreateOrGetModelInvocation() error = %v", err)
	}
	if _, err = first.TransitionModelInvocation(ctx, invocation.ID, InvocationPending, InvocationTransition{Status: InvocationRunning}); err != nil {
		t.Fatalf("TransitionModelInvocation() error = %v", err)
	}
	if _, _, err = first.SaveCheckpoint(ctx, CheckpointSpec{
		ID: "checkpoint-1", RunID: "run-1", CommandID: command.ID, SessionID: "session-1",
		Sequence: 3, State: json.RawMessage(`{"stage":"assessing"}`),
	}); err != nil {
		t.Fatalf("SaveCheckpoint() error = %v", err)
	}
	if _, acquired, err := first.AcquireRunLease(ctx, "run-1", "worker-1", "token-1", now, time.Minute); err != nil || !acquired {
		t.Fatalf("AcquireRunLease() = acquired %v error %v", acquired, err)
	}
	if _, _, err = first.EnqueueOutbox(ctx, OutboxMessageSpec{
		ID: "outbox-1", SessionID: "session-1", Sequence: 1, Type: "answer.accepted",
		Payload: json.RawMessage(`{"eventId":"event-1"}`), AvailableAt: now,
	}); err != nil {
		t.Fatalf("EnqueueOutbox() error = %v", err)
	}
	if err = first.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}

	second, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatalf("second OpenSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })
	loadedCommand, err := second.GetCommand(ctx, command.ID)
	if err != nil || loadedCommand.Status != CommandRunning {
		t.Fatalf("GetCommand(after reopen) = status %q error %v", loadedCommand.Status, err)
	}
	events, err := second.ListSessionEvents(ctx, "session-1", 0, 10)
	if err != nil || len(events) != 1 || events[0].EventID != "event-1" {
		t.Fatalf("ListSessionEvents(after reopen) = %#v error %v", events, err)
	}
	loadedInvocation, err := second.GetModelInvocation(ctx, invocation.ID)
	if err != nil || loadedInvocation.Status != InvocationRunning {
		t.Fatalf("GetModelInvocation(after reopen) = status %q error %v", loadedInvocation.Status, err)
	}
	recoverable, err := second.ListRecoverableModelInvocations(ctx, 10)
	if err != nil || len(recoverable) != 1 || recoverable[0].ID != invocation.ID {
		t.Fatalf("ListRecoverableModelInvocations(after reopen) = %#v error %v", recoverable, err)
	}
	checkpoint, err := second.LoadLatestCheckpoint(ctx, "run-1")
	if err != nil || checkpoint.Sequence != 3 {
		t.Fatalf("LoadLatestCheckpoint(after reopen) = sequence %d error %v", checkpoint.Sequence, err)
	}
	lease, err := second.GetRunLease(ctx, "run-1")
	if err != nil || lease.OwnerID != "worker-1" {
		t.Fatalf("GetRunLease(after reopen) = owner %q error %v", lease.OwnerID, err)
	}
	message, err := second.GetOutboxMessage(ctx, "outbox-1")
	if err != nil || message.Status != OutboxPending {
		t.Fatalf("GetOutboxMessage(after reopen) = status %q error %v", message.Status, err)
	}
}

func TestSQLitePersistenceRunLeaseExpiryAndOwnership(t *testing.T) {
	store := openTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	first, acquired, err := store.AcquireRunLease(ctx, "run-1", "worker-1", "token-1", now, time.Minute)
	if err != nil || !acquired || first.OwnerID != "worker-1" {
		t.Fatalf("first AcquireRunLease() = %#v acquired %v error %v", first, acquired, err)
	}
	held, acquired, err := store.AcquireRunLease(ctx, "run-1", "worker-2", "token-2", now.Add(30*time.Second), time.Minute)
	if !errors.Is(err, ErrLeaseHeld) || acquired || held.OwnerID != "worker-1" {
		t.Fatalf("held AcquireRunLease() = owner %q acquired %v error %v", held.OwnerID, acquired, err)
	}
	if _, err = store.RenewRunLease(ctx, "run-1", "worker-2", "token-2", now.Add(30*time.Second), time.Minute); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("foreign RenewRunLease() error = %v, want ErrLeaseLost", err)
	}
	taken, acquired, err := store.AcquireRunLease(ctx, "run-1", "worker-2", "token-2", now.Add(time.Minute), time.Minute)
	if err != nil || !acquired || taken.OwnerID != "worker-2" {
		t.Fatalf("expired AcquireRunLease() = owner %q acquired %v error %v", taken.OwnerID, acquired, err)
	}
	if err = store.ReleaseRunLease(ctx, "run-1", "worker-1", "token-1"); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale ReleaseRunLease() error = %v, want ErrLeaseLost", err)
	}
	if err = store.ReleaseRunLease(ctx, "run-1", "worker-2", "token-2"); err != nil {
		t.Fatalf("owner ReleaseRunLease() error = %v", err)
	}
}

func TestSQLitePersistenceOutboxRecoversExpiredClaimsAfterReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "interviews.sqlite")
	ctx := context.Background()
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	first, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatalf("first OpenSQLiteStore() error = %v", err)
	}
	for sequence := int64(1); sequence <= 2; sequence++ {
		if _, _, err = first.EnqueueOutbox(ctx, OutboxMessageSpec{
			ID: fmt.Sprintf("outbox-%d", sequence), SessionID: "session-1", Sequence: sequence,
			Type: "session.event", Payload: json.RawMessage(fmt.Sprintf(`{"sequence":%d}`, sequence)),
			AvailableAt: now,
		}); err != nil {
			t.Fatalf("EnqueueOutbox(%d) error = %v", sequence, err)
		}
	}
	claimed, err := first.ClaimOutbox(ctx, "dispatcher-1", now, time.Minute, 2)
	if err != nil || len(claimed) != 2 {
		t.Fatalf("first ClaimOutbox() = %d messages error %v", len(claimed), err)
	}
	if err = first.MarkOutboxPublished(ctx, claimed[0].ID, "dispatcher-1", now.Add(time.Second)); err != nil {
		t.Fatalf("MarkOutboxPublished() error = %v", err)
	}
	unpublishedID := claimed[1].ID
	if err = first.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}

	second, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatalf("second OpenSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })
	recovered, err := second.ClaimOutbox(ctx, "dispatcher-2", now.Add(time.Minute), time.Minute, 10)
	if err != nil || len(recovered) != 1 {
		t.Fatalf("recovery ClaimOutbox() = %#v error %v", recovered, err)
	}
	if recovered[0].ID != unpublishedID || recovered[0].Attempts != 2 {
		t.Fatalf("recovered message = id %q attempts %d, want %q/2", recovered[0].ID, recovered[0].Attempts, unpublishedID)
	}
	retryAt := now.Add(3 * time.Minute)
	if err = second.MarkOutboxFailed(ctx, unpublishedID, "dispatcher-2", "temporary failure", retryAt); err != nil {
		t.Fatalf("MarkOutboxFailed() error = %v", err)
	}
	if early, claimErr := second.ClaimOutbox(ctx, "dispatcher-3", now.Add(2*time.Minute), time.Minute, 10); claimErr != nil || len(early) != 0 {
		t.Fatalf("early ClaimOutbox() = %#v error %v", early, claimErr)
	}
	retried, err := second.ClaimOutbox(ctx, "dispatcher-3", retryAt, time.Minute, 10)
	if err != nil || len(retried) != 1 || retried[0].ID != unpublishedID {
		t.Fatalf("retry ClaimOutbox() = %#v error %v", retried, err)
	}
	if err = second.MarkOutboxPublished(ctx, unpublishedID, "dispatcher-3", retryAt.Add(time.Second)); err != nil {
		t.Fatalf("second MarkOutboxPublished() error = %v", err)
	}
	if remaining, claimErr := second.ClaimOutbox(ctx, "dispatcher-4", retryAt.Add(10*time.Minute), time.Minute, 10); claimErr != nil || len(remaining) != 0 {
		t.Fatalf("published messages were reclaimed: %#v error %v", remaining, claimErr)
	}
}

func TestSQLiteCommitAnswerRollsBackSessionAndCommandWhenEventFails(t *testing.T) {
	store := openTestSQLiteStore(t)
	ctx := context.Background()
	session := sqliteTestSession()
	if err := store.Create(ctx, session); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	command, _, err := store.CreateOrGetCommand(ctx, CommandSpec{
		ID: "command-answer", PrincipalID: session.ClientSessionID, SessionID: session.ID,
		Action: string(ActionAnswer), IdempotencyKey: "answer-key", SubjectID: session.CurrentQuestion.ID,
		RequestHash: "sha256:answer",
	})
	if err != nil {
		t.Fatalf("CreateOrGetCommand() error = %v", err)
	}
	command, err = store.TransitionCommand(ctx, command.ID, CommandPending, CommandTransition{Status: CommandRunning})
	if err != nil {
		t.Fatalf("TransitionCommand() error = %v", err)
	}
	if _, _, err = store.AppendSessionEvent(ctx, SessionEventSpec{
		EventID: "event-collision", SessionID: session.ID, CommandID: command.ID,
		Type: "answer.started", Payload: json.RawMessage(`{"questionId":"question-1"}`),
	}); err != nil {
		t.Fatalf("AppendSessionEvent() error = %v", err)
	}

	expectedVersion := session.Version
	session.Version++
	session.State = StateCompleted
	session.CurrentQuestion = nil
	err = store.CommitAnswer(ctx, AnswerCommitSpec{
		Session: session, ExpectedVersion: expectedVersion, CommandID: command.ID,
		Result: json.RawMessage(`{"interviewId":"interview-1","state":"completed"}`),
		Event: SessionEventSpec{
			EventID: "event-collision", SessionID: session.ID, CommandID: command.ID,
			Type: "answer.committed", Payload: json.RawMessage(`{"questionId":"question-1","status":"succeeded"}`),
		},
	})
	if err == nil {
		t.Fatal("CommitAnswer() error = nil, want duplicate event failure")
	}
	loaded, loadErr := store.Load(ctx, session.ID)
	if loadErr != nil || loaded.Version != expectedVersion || loaded.State != StateAwaitingAnswer || loaded.CurrentQuestion == nil {
		t.Fatalf("session was partially committed: version %d state %q question %+v error %v", loaded.Version, loaded.State, loaded.CurrentQuestion, loadErr)
	}
	command, err = store.GetCommand(ctx, command.ID)
	if err != nil || command.Status != CommandRunning || len(command.Result) != 0 {
		t.Fatalf("command was partially committed: status %q result %s error %v", command.Status, command.Result, err)
	}
	if count := sqliteRowCount(t, store, "interview_session_events"); count != 1 {
		t.Fatalf("event rows = %d, want only the pre-existing event", count)
	}
}

func openSharedSQLiteStores(t *testing.T) (*SQLiteStore, *SQLiteStore) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "interviews.sqlite")
	left, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatalf("first OpenSQLiteStore() error = %v", err)
	}
	right, err := OpenSQLiteStore(path)
	if err != nil {
		_ = left.Close()
		t.Fatalf("second OpenSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = left.Close()
		_ = right.Close()
	})
	return left, right
}

func sqliteRowCount(t *testing.T, store *SQLiteStore, table string) int {
	t.Helper()
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
		t.Fatalf("count %s rows error = %v", table, err)
	}
	return count
}
