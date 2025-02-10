package session

import (
	"sync"
	pb "tss-demo/proto"
)

// Session represents a single TSS protocol session
type Session struct {
	SessionID  string
	partyCount int
	partyIDs   []uint16
	ready      bool
	readyCh    chan struct{}
	msgCh      chan *pb.TSSMessage
	done       chan struct{}
	mu         sync.RWMutex
}

// SessionManager manages multiple TSS sessions
type SessionManager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

// NewSessionManager creates a new SessionManager
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
	}
}

// CreateSession creates a new TSS session
func (sm *SessionManager) CreateSession(sessionID string, partyCount int) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session := &Session{
		SessionID:  sessionID,
		partyCount: partyCount,
		readyCh:    make(chan struct{}),
		msgCh:      make(chan *pb.TSSMessage, partyCount),
		done:       make(chan struct{}),
	}

	sm.sessions[sessionID] = session
	return session
}

// GetSession returns a session by ID
func (sm *SessionManager) GetSession(sessionID string) (*Session, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	session, exists := sm.sessions[sessionID]
	return session, exists
}

// DeleteSession deletes a session by ID
func (sm *SessionManager) DeleteSession(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, sessionID)
}

// AddParty adds a party ID to the session
func (s *Session) AddParty(partyID uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Tránh thêm trùng lặp
	for _, id := range s.partyIDs {
		if id == partyID {
			return
		}
	}

	s.partyIDs = append(s.partyIDs, partyID)
	if len(s.partyIDs) >= s.partyCount {
		s.ready = true
		close(s.readyCh) // Thông báo rằng session đã sẵn sàng
	}
}

// IsReady checks if the session is ready
func (s *Session) IsReady() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ready
}

// ListSessions returns all active session IDs
func (sm *SessionManager) ListSessions() []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	sessionIDs := make([]string, 0, len(sm.sessions))
	for id := range sm.sessions {
		sessionIDs = append(sessionIDs, id)
	}
	return sessionIDs
}
