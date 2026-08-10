package session

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type Session struct {
	ID        string            `json:"id"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
	Values    map[string]string `json:"values,omitempty"`
}

type Store struct {
	mu       sync.RWMutex
	sessions map[string]Session
}

func NewStore() *Store {
	return &Store{sessions: make(map[string]Session)}
}

// Create accepts an optional caller-provided ID and otherwise creates a
// cryptographically random one.
func (s *Store) Create(ids ...string) (Session, error) {
	if s == nil {
		return Session{}, errors.New("session: nil store")
	}
	id := ""
	if len(ids) > 0 {
		id = strings.TrimSpace(ids[0])
	}
	if id == "" {
		id = newID()
	}
	now := time.Now().UTC()
	created := Session{ID: id, CreatedAt: now, UpdatedAt: now, Values: make(map[string]string)}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.sessions[id]; exists {
		return Session{}, errors.New("session: id already exists")
	}
	s.sessions[id] = created
	return cloneSession(created), nil
}

func (s *Store) Get(id string) (Session, bool) {
	if s == nil {
		return Session{}, false
	}
	s.mu.RLock()
	stored, exists := s.sessions[id]
	s.mu.RUnlock()
	if !exists {
		return Session{}, false
	}
	return cloneSession(stored), true
}

func (s *Store) SetValue(id, key, value string) error {
	if s == nil {
		return errors.New("session: nil store")
	}
	if strings.TrimSpace(key) == "" {
		return errors.New("session: value key is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, exists := s.sessions[id]
	if !exists {
		return errors.New("session: id does not exist")
	}
	if stored.Values == nil {
		stored.Values = make(map[string]string)
	}
	stored.Values[key] = value
	stored.UpdatedAt = time.Now().UTC()
	s.sessions[id] = stored
	return nil
}

func (s *Store) Delete(id string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.sessions[id]; !exists {
		return false
	}
	delete(s.sessions, id)
	return true
}

func (s *Store) IDs() []string {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	result := make([]string, 0, len(s.sessions))
	for id := range s.sessions {
		result = append(result, id)
	}
	s.mu.RUnlock()
	sort.Strings(result)
	return result
}

func cloneSession(session Session) Session {
	session.Values = cloneMetadata(session.Values)
	return session
}

func newID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err == nil {
		return hex.EncodeToString(buffer)
	}
	return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
}
