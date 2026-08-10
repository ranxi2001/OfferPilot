// Package session owns concurrency-safe, session-scoped conversational state.
package session

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type Message struct {
	Role     string            `json:"role"`
	Content  string            `json:"content"`
	At       time.Time         `json:"at"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type Memory struct {
	mu          sync.RWMutex
	bySession   map[string][]Message
	maxMessages int
}

// NewMemory creates isolated per-session histories. maxMessages <= 0 means
// unbounded retention; callers normally use a bounded value such as 100.
func NewMemory(maxMessages int) *Memory {
	return &Memory{
		bySession:   make(map[string][]Message),
		maxMessages: maxMessages,
	}
}

func (m *Memory) Append(sessionID string, message Message) error {
	if m == nil {
		return errors.New("session: nil memory")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session: session id is required")
	}
	if message.At.IsZero() {
		message.At = time.Now().UTC()
	}
	message.Metadata = cloneMetadata(message.Metadata)

	m.mu.Lock()
	history := append(m.bySession[sessionID], message)
	if m.maxMessages > 0 && len(history) > m.maxMessages {
		trimmed := make([]Message, m.maxMessages)
		copy(trimmed, history[len(history)-m.maxMessages:])
		history = trimmed
	}
	m.bySession[sessionID] = history
	m.mu.Unlock()
	return nil
}

func (m *Memory) Add(sessionID, role, content string) error {
	return m.Append(sessionID, Message{Role: role, Content: content})
}

// Messages returns a deep-enough defensive copy: neither the slice nor any
// message metadata can be used to mutate stored state.
func (m *Memory) Messages(sessionID string) []Message {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	history := m.bySession[sessionID]
	result := cloneMessages(history)
	m.mu.RUnlock()
	return result
}

func (m *Memory) Replace(sessionID string, messages []Message) error {
	if m == nil {
		return errors.New("session: nil memory")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session: session id is required")
	}
	copyMessages := cloneMessages(messages)
	if m.maxMessages > 0 && len(copyMessages) > m.maxMessages {
		copyMessages = copyMessages[len(copyMessages)-m.maxMessages:]
	}
	m.mu.Lock()
	m.bySession[sessionID] = copyMessages
	m.mu.Unlock()
	return nil
}

func (m *Memory) Clear(sessionID string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	delete(m.bySession, sessionID)
	m.mu.Unlock()
}

func (m *Memory) SessionIDs() []string {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	result := make([]string, 0, len(m.bySession))
	for sessionID := range m.bySession {
		result = append(result, sessionID)
	}
	m.mu.RUnlock()
	sort.Strings(result)
	return result
}

func cloneMessages(messages []Message) []Message {
	if len(messages) == 0 {
		return nil
	}
	result := make([]Message, len(messages))
	for idx, message := range messages {
		result[idx] = message
		result[idx].Metadata = cloneMetadata(message.Metadata)
	}
	return result
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if metadata == nil {
		return nil
	}
	result := make(map[string]string, len(metadata))
	for key, value := range metadata {
		result[key] = value
	}
	return result
}
