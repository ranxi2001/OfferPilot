package interview

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	_ "modernc.org/sqlite"
)

const sqliteBusyTimeout = 5 * time.Second

type SQLiteStore struct {
	db *sql.DB
}

// OpenSQLiteStore opens a SQLite database, configures it for server use, and
// applies all interview-store schema migrations.
func OpenSQLiteStore(path string) (*SQLiteStore, error) {
	if path == "" {
		return nil, errors.New("interview sqlite store: path is empty")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("interview sqlite store: open: %w", err)
	}
	store, err := NewSQLiteStore(db)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

// NewSQLiteStore configures an existing database handle and applies schema
// migrations. SQLite writes are intentionally serialized through one pooled
// connection; optimistic versions still arbitrate competing aggregate saves.
func NewSQLiteStore(db *sql.DB) (*SQLiteStore, error) {
	if db == nil {
		return nil, errors.New("interview sqlite store: database is nil")
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), sqliteBusyTimeout)
	defer cancel()
	for _, pragma := range []string{
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA foreign_keys = ON",
	} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			return nil, fmt.Errorf("interview sqlite store: configure %q: %w", pragma, err)
		}
	}

	store := &SQLiteStore{db: db}
	if err := store.migrate(ctx); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) Create(ctx context.Context, session InterviewSession) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	payload, err := marshalSQLiteSession(session)
	if err != nil {
		return err
	}
	now := time.Now().UTC().UnixMilli()
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO interview_sessions (
			id, version, snapshot_json, created_at_ms, updated_at_ms
		) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`, session.ID, session.Version, payload, now, now)
	if err != nil {
		return fmt.Errorf("interview sqlite store: create %q: %w", session.ID, err)
	}
	created, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("interview sqlite store: inspect create %q: %w", session.ID, err)
	}
	if created != 1 {
		return ErrStoreConflict
	}
	return nil
}

func (s *SQLiteStore) Load(ctx context.Context, id string) (InterviewSession, error) {
	if err := ctx.Err(); err != nil {
		return InterviewSession{}, err
	}
	var (
		version int64
		payload []byte
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT version, snapshot_json
		FROM interview_sessions
		WHERE id = ?
	`, id).Scan(&version, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return InterviewSession{}, ErrStoreNotFound
	}
	if err != nil {
		return InterviewSession{}, fmt.Errorf("interview sqlite store: load %q: %w", id, err)
	}
	session, err := unmarshalSQLiteSession(payload)
	if err != nil {
		return InterviewSession{}, fmt.Errorf("interview sqlite store: load %q: %w", id, err)
	}
	if session.ID != id || session.Version != version {
		return InterviewSession{}, fmt.Errorf(
			"interview sqlite store: corrupt snapshot %q (snapshot id=%q version=%d, row version=%d)",
			id,
			session.ID,
			session.Version,
			version,
		)
	}
	return session, nil
}

func (s *SQLiteStore) Save(ctx context.Context, session InterviewSession, expectedVersion int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if expectedVersion == math.MaxInt64 || session.Version != expectedVersion+1 {
		return s.classifySaveMiss(ctx, session.ID)
	}
	payload, err := marshalSQLiteSession(session)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE interview_sessions
		SET version = ?, snapshot_json = ?, updated_at_ms = ?
		WHERE id = ? AND version = ?
	`, session.Version, payload, time.Now().UTC().UnixMilli(), session.ID, expectedVersion)
	if err != nil {
		return fmt.Errorf("interview sqlite store: save %q: %w", session.ID, err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("interview sqlite store: inspect save %q: %w", session.ID, err)
	}
	if updated == 1 {
		return nil
	}

	return s.classifySaveMiss(ctx, session.ID)
}

func (s *SQLiteStore) classifySaveMiss(ctx context.Context, id string) error {
	var exists int
	err := s.db.QueryRowContext(ctx, `
		SELECT 1 FROM interview_sessions WHERE id = ?
	`, id).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrStoreNotFound
	}
	if err != nil {
		return fmt.Errorf("interview sqlite store: inspect missing save %q: %w", id, err)
	}
	return ErrStoreConflict
}

type sqliteSessionSnapshot struct {
	Session        InterviewSession  `json:"session"`
	SourceContents map[string]string `json:"sourceContents,omitempty"`
}

func marshalSQLiteSession(session InterviewSession) ([]byte, error) {
	contents := make(map[string]string, len(session.Sources.Documents))
	for id, document := range session.Sources.Documents {
		contents[id] = document.Content
	}
	payload, err := json.Marshal(sqliteSessionSnapshot{
		Session:        session,
		SourceContents: contents,
	})
	if err != nil {
		return nil, fmt.Errorf("interview sqlite store: encode snapshot: %w", err)
	}
	return payload, nil
}

func unmarshalSQLiteSession(payload []byte) (InterviewSession, error) {
	var snapshot sqliteSessionSnapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return InterviewSession{}, fmt.Errorf("decode snapshot: %w", err)
	}
	for id, content := range snapshot.SourceContents {
		document, ok := snapshot.Session.Sources.Documents[id]
		if !ok {
			continue
		}
		document.Content = content
		snapshot.Session.Sources.Documents[id] = document
	}
	return snapshot.Session, nil
}

type sqliteMigration struct {
	version    int
	statements []string
}

var sqliteMigrations = []sqliteMigration{
	{
		version: 1,
		statements: []string{`
			CREATE TABLE IF NOT EXISTS interview_sessions (
				id TEXT PRIMARY KEY,
				version INTEGER NOT NULL CHECK (version >= 0),
				snapshot_json BLOB NOT NULL,
				created_at_ms INTEGER NOT NULL,
				updated_at_ms INTEGER NOT NULL
			)
		`},
	},
	{
		version: 2,
		statements: []string{
			`CREATE TABLE IF NOT EXISTS interview_commands (
				id TEXT PRIMARY KEY,
				session_id TEXT NOT NULL,
				action TEXT NOT NULL,
				idempotency_key TEXT NOT NULL,
				subject_id TEXT NOT NULL DEFAULT '',
				request_hash TEXT NOT NULL,
				status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'succeeded', 'failed')),
				result_json BLOB,
				error_json BLOB,
				created_at_ms INTEGER NOT NULL,
				updated_at_ms INTEGER NOT NULL,
				UNIQUE (session_id, idempotency_key)
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS interview_commands_subject_unique
				ON interview_commands (session_id, action, subject_id)
				WHERE subject_id <> ''`,
			`CREATE INDEX IF NOT EXISTS interview_commands_status_idx
				ON interview_commands (status, updated_at_ms)`,
			`CREATE TABLE IF NOT EXISTS interview_session_cursors (
				session_id TEXT PRIMARY KEY,
				last_sequence INTEGER NOT NULL CHECK (last_sequence >= 0)
			)`,
			`CREATE TABLE IF NOT EXISTS interview_session_events (
				session_id TEXT NOT NULL,
				sequence INTEGER NOT NULL CHECK (sequence > 0),
				event_id TEXT NOT NULL UNIQUE,
				command_id TEXT NOT NULL DEFAULT '',
				event_type TEXT NOT NULL,
				payload_json BLOB NOT NULL,
				created_at_ms INTEGER NOT NULL,
				PRIMARY KEY (session_id, sequence)
			)`,
			`CREATE INDEX IF NOT EXISTS interview_session_events_command_idx
				ON interview_session_events (command_id) WHERE command_id <> ''`,
			`CREATE TABLE IF NOT EXISTS interview_model_invocations (
				id TEXT PRIMARY KEY,
				run_id TEXT NOT NULL,
				command_id TEXT NOT NULL DEFAULT '',
				session_id TEXT NOT NULL,
				agent TEXT NOT NULL,
				request_hash TEXT NOT NULL,
				status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'manual_review')),
				attempt INTEGER NOT NULL CHECK (attempt > 0),
				result_json BLOB,
				error_json BLOB,
				created_at_ms INTEGER NOT NULL,
				updated_at_ms INTEGER NOT NULL,
				UNIQUE (run_id, request_hash, attempt)
			)`,
			`CREATE INDEX IF NOT EXISTS interview_model_invocations_recovery_idx
				ON interview_model_invocations (status, updated_at_ms)`,
			`CREATE TABLE IF NOT EXISTS interview_checkpoints (
				id TEXT PRIMARY KEY,
				run_id TEXT NOT NULL,
				command_id TEXT NOT NULL DEFAULT '',
				session_id TEXT NOT NULL,
				sequence INTEGER NOT NULL CHECK (sequence >= 0),
				state_json BLOB NOT NULL,
				created_at_ms INTEGER NOT NULL,
				UNIQUE (run_id, sequence)
			)`,
			`CREATE INDEX IF NOT EXISTS interview_checkpoints_latest_idx
				ON interview_checkpoints (run_id, sequence DESC)`,
			`CREATE TABLE IF NOT EXISTS interview_run_leases (
				run_id TEXT PRIMARY KEY,
				owner_id TEXT NOT NULL,
				lease_token TEXT NOT NULL,
				expires_at_ms INTEGER NOT NULL,
				updated_at_ms INTEGER NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS interview_run_leases_expiry_idx
				ON interview_run_leases (expires_at_ms)`,
			`CREATE TABLE IF NOT EXISTS interview_outbox (
				id TEXT PRIMARY KEY,
				session_id TEXT NOT NULL,
				sequence INTEGER NOT NULL CHECK (sequence > 0),
				event_type TEXT NOT NULL,
				payload_json BLOB NOT NULL,
				status TEXT NOT NULL CHECK (status IN ('pending', 'claimed', 'published')),
				available_at_ms INTEGER NOT NULL,
				claim_owner TEXT NOT NULL DEFAULT '',
				claim_until_ms INTEGER,
				attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
				last_error TEXT NOT NULL DEFAULT '',
				created_at_ms INTEGER NOT NULL,
				published_at_ms INTEGER,
				UNIQUE (session_id, sequence, event_type)
			)`,
			`CREATE INDEX IF NOT EXISTS interview_outbox_dispatch_idx
				ON interview_outbox (status, available_at_ms, claim_until_ms, created_at_ms)`,
		},
	},
}

func (s *SQLiteStore) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS interview_store_migrations (
			version INTEGER PRIMARY KEY,
			applied_at_ms INTEGER NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("interview sqlite store: create migration table: %w", err)
	}

	for _, migration := range sqliteMigrations {
		if err := s.applyMigration(ctx, migration); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) applyMigration(ctx context.Context, migration sqliteMigration) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("interview sqlite store: begin migration %d: %w", migration.version, err)
	}
	defer func() { _ = tx.Rollback() }()

	var applied int
	err = tx.QueryRowContext(ctx, `
		SELECT 1 FROM interview_store_migrations WHERE version = ?
	`, migration.version).Scan(&applied)
	if err == nil {
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("interview sqlite store: check migration %d: %w", migration.version, err)
	}
	for _, statement := range migration.statements {
		if _, err = tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("interview sqlite store: apply migration %d: %w", migration.version, err)
		}
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO interview_store_migrations (version, applied_at_ms)
		VALUES (?, ?)
	`, migration.version, time.Now().UTC().UnixMilli()); err != nil {
		return fmt.Errorf("interview sqlite store: record migration %d: %w", migration.version, err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("interview sqlite store: commit migration %d: %w", migration.version, err)
	}
	return nil
}
