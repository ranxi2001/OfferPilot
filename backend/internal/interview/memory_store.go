package interview

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type MemoryStore struct {
	mu       sync.RWMutex
	sessions map[string]InterviewSession
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{sessions: make(map[string]InterviewSession)}
}

func (s *MemoryStore) Create(ctx context.Context, session InterviewSession) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	copy := cloneSession(session)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.sessions[session.ID]; exists {
		return ErrStoreConflict
	}
	s.sessions[session.ID] = copy
	return nil
}

func (s *MemoryStore) Load(ctx context.Context, id string) (InterviewSession, error) {
	if err := ctx.Err(); err != nil {
		return InterviewSession{}, err
	}
	s.mu.RLock()
	session, exists := s.sessions[id]
	s.mu.RUnlock()
	if !exists {
		return InterviewSession{}, ErrStoreNotFound
	}
	return cloneSession(session), nil
}

func (s *MemoryStore) Save(ctx context.Context, session InterviewSession, expectedVersion int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	copy := cloneSession(session)
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.sessions[session.ID]
	if !exists {
		return ErrStoreNotFound
	}
	if current.Version != expectedVersion || session.Version != expectedVersion+1 {
		return ErrStoreConflict
	}
	s.sessions[session.ID] = copy
	return nil
}

func cloneSession(session InterviewSession) InterviewSession {
	data, err := json.Marshal(session)
	if err != nil {
		panic(fmt.Sprintf("interview: clone session: %v", err))
	}
	var clone InterviewSession
	if err := json.Unmarshal(data, &clone); err != nil {
		panic(fmt.Sprintf("interview: clone session: %v", err))
	}
	// SourceDocument.Content is deliberately excluded from JSON responses, but
	// it remains part of the internal source index across store snapshots.
	for id, document := range session.Sources.Documents {
		clone.Sources.Documents[id] = document
	}
	return clone
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

type randomIDGenerator struct{}

func (randomIDGenerator) NewID(prefix string) string {
	var bytes [12]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(bytes[:])
}
