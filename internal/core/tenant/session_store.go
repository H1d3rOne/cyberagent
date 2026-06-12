package tenant

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	ErrSessionStoreDisabled = errors.New("web session store is disabled")
	ErrSessionNotFound      = errors.New("web session not found")
)

// SessionStore is the future server-side store for browser/API auth sessions.
// It is intentionally separate from Agent sessions and runtime task/run/turn
// state. L3 keeps this store detached from the local daemon router until real
// Web login/session middleware is explicitly implemented.
type SessionStore interface {
	CreateSession(ctx context.Context, session Session) (Session, error)
	GetSession(ctx context.Context, sessionID string) (Session, bool, error)
	RevokeSession(ctx context.Context, sessionID string, revokedAt time.Time) (Session, bool, error)
}

// DisabledSessionStore is the default local-mode placeholder. Calling it is a
// programming/configuration error; local daemon auth continues to use bearer
// tokens and never consults this store.
type DisabledSessionStore struct{}

func (DisabledSessionStore) CreateSession(context.Context, Session) (Session, error) {
	return Session{}, ErrSessionStoreDisabled
}

func (DisabledSessionStore) GetSession(context.Context, string) (Session, bool, error) {
	return Session{}, false, ErrSessionStoreDisabled
}

func (DisabledSessionStore) RevokeSession(context.Context, string, time.Time) (Session, bool, error) {
	return Session{}, false, ErrSessionStoreDisabled
}

// MemorySessionStore is a dependency-light server-side session store suitable
// for tests and future single-process Web auth bootstrap work. It is not wired
// into the local daemon router by default.
type MemorySessionStore struct {
	mu       sync.RWMutex
	sessions map[string]Session
}

func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{sessions: make(map[string]Session)}
}

func (s *MemorySessionStore) CreateSession(ctx context.Context, session Session) (Session, error) {
	if err := ctxErr(ctx); err != nil {
		return Session{}, err
	}
	if s == nil {
		return Session{}, errors.New("web session store is nil")
	}
	session, err := normalizeSessionForStore(session)
	if err != nil {
		return Session{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions == nil {
		s.sessions = make(map[string]Session)
	}
	s.sessions[session.ID] = session.Clone()
	return session.Clone(), nil
}

func (s *MemorySessionStore) GetSession(ctx context.Context, sessionID string) (Session, bool, error) {
	if err := ctxErr(ctx); err != nil {
		return Session{}, false, err
	}
	if s == nil {
		return Session{}, false, errors.New("web session store is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return Session{}, false, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return Session{}, false, nil
	}
	return session.Clone(), true, nil
}

func (s *MemorySessionStore) RevokeSession(ctx context.Context, sessionID string, revokedAt time.Time) (Session, bool, error) {
	if err := ctxErr(ctx); err != nil {
		return Session{}, false, err
	}
	if s == nil {
		return Session{}, false, errors.New("web session store is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return Session{}, false, nil
	}
	if revokedAt.IsZero() {
		revokedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return Session{}, false, nil
	}
	session = session.Clone()
	session.RevokedAt = &revokedAt
	s.sessions[sessionID] = session.Clone()
	return session.Clone(), true, nil
}

// ActiveAt reports whether a stored session is usable at the supplied time. It
// is a helper for future auth middleware; current local bearer requests do not
// use HTTP sessions.
func (s Session) ActiveAt(now time.Time) bool {
	if strings.TrimSpace(s.ID) == "" || strings.TrimSpace(s.SubjectID) == "" || strings.TrimSpace(s.AuthMethod) == "" {
		return false
	}
	if s.RevokedAt != nil {
		return false
	}
	if !s.ExpiresAt.IsZero() {
		if now.IsZero() {
			now = time.Now().UTC()
		}
		if !now.Before(s.ExpiresAt) {
			return false
		}
	}
	return true
}

func normalizeSessionForStore(session Session) (Session, error) {
	session = session.Clone()
	session.ID = strings.TrimSpace(session.ID)
	session.SubjectID = strings.TrimSpace(session.SubjectID)
	session.AuthMethod = strings.TrimSpace(session.AuthMethod)
	session.TenantID = strings.TrimSpace(session.TenantID)
	session.WorkspaceID = strings.TrimSpace(session.WorkspaceID)
	if session.ID == "" {
		return Session{}, errors.New("web session id is required")
	}
	if session.SubjectID == "" {
		return Session{}, errors.New("web session subject_id is required")
	}
	if session.AuthMethod == "" {
		return Session{}, errors.New("web session auth_method is required")
	}
	for _, role := range session.Roles {
		if !role.Valid() {
			return Session{}, fmt.Errorf("web session contains unknown role %q", role)
		}
	}
	return session, nil
}

func ctxErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
