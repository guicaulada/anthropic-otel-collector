package anthropicreceiver

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"
)

// SessionContext holds session information attached to each request's telemetry.
type SessionContext struct {
	// SessionID uniquely identifies this session.
	SessionID string

	// ProjectPath is the working directory for this session.
	ProjectPath string

	// ProjectName is the base name of the project directory.
	ProjectName string

	// UserID is the user identifier, if available.
	UserID string

	// RequestNumber is the 1-based sequence number of this request within the session.
	RequestNumber int

	// IsNewSession indicates this is the first request of a new session.
	IsNewSession bool
}

// sessionState tracks the state of an active session.
type sessionState struct {
	id                string
	startTime         time.Time
	lastRequestTime   time.Time
	requestCount      int
	peakConversation  int
	cumulativeCost    float64
	cumulativeInput   int
	cumulativeOutput  int
	cumulativeDuration time.Duration
}

// sessionKey uniquely identifies a session by API key hash and project path.
type sessionKey struct {
	apiKeyHash  string
	projectPath string
}

// sessionTracker manages in-memory session state for Claude Code requests.
type sessionTracker struct {
	mu       sync.Mutex
	sessions map[sessionKey]*sessionState
	timeout  time.Duration
	done     chan struct{}
}

// newSessionTracker creates a new session tracker with the given timeout.
func newSessionTracker(timeout time.Duration) *sessionTracker {
	st := &sessionTracker{
		sessions: make(map[sessionKey]*sessionState),
		timeout:  timeout,
		done:     make(chan struct{}),
	}
	go st.cleanupLoop()
	return st
}

// TrackRequest processes a request and returns session context for telemetry.
// It determines whether this is a new session or continues an existing one.
func (st *sessionTracker) TrackRequest(
	apiKeyHash string,
	ccCtx ClaudeCodeContext,
	cost float64,
	inputTokens int,
	outputTokens int,
	duration time.Duration,
) SessionContext {
	st.mu.Lock()
	defer st.mu.Unlock()

	key := sessionKey{
		apiKeyHash:  apiKeyHash,
		projectPath: ccCtx.WorkingDir,
	}

	now := time.Now()
	existing, ok := st.sessions[key]

	isNew := !ok ||
		now.Sub(existing.lastRequestTime) > st.timeout ||
		isConversationReset(ccCtx.ConversationDepth, existing.peakConversation)

	if isNew {
		existing = &sessionState{
			id:        generateSessionID(),
			startTime: now,
		}
		st.sessions[key] = existing
	}

	// Update session state
	existing.lastRequestTime = now
	existing.requestCount++
	existing.cumulativeCost += cost
	existing.cumulativeInput += inputTokens
	existing.cumulativeOutput += outputTokens
	existing.cumulativeDuration += duration
	if ccCtx.ConversationDepth > existing.peakConversation {
		existing.peakConversation = ccCtx.ConversationDepth
	}

	return SessionContext{
		SessionID:     existing.id,
		ProjectPath:   ccCtx.WorkingDir,
		ProjectName:   ccCtx.ProjectName,
		UserID:        ccCtx.UserID,
		RequestNumber: existing.requestCount,
		IsNewSession:  isNew,
	}
}

// isConversationReset detects a new conversation by checking if the conversation
// depth dropped significantly below the peak (below half the peak).
func isConversationReset(currentDepth, peakDepth int) bool {
	if peakDepth <= 2 {
		return false
	}
	return currentDepth < peakDepth/2
}

// Stop stops the cleanup goroutine.
func (st *sessionTracker) Stop() {
	close(st.done)
}

// cleanupLoop periodically removes expired sessions.
func (st *sessionTracker) cleanupLoop() {
	ticker := time.NewTicker(st.timeout / 2)
	defer ticker.Stop()

	for {
		select {
		case <-st.done:
			return
		case <-ticker.C:
			st.cleanup()
		}
	}
}

// cleanup removes sessions that have timed out.
func (st *sessionTracker) cleanup() {
	st.mu.Lock()
	defer st.mu.Unlock()

	now := time.Now()
	for key, session := range st.sessions {
		if now.Sub(session.lastRequestTime) > st.timeout {
			delete(st.sessions, key)
		}
	}
}

// activeSessionCount returns the number of active sessions (for testing).
func (st *sessionTracker) activeSessionCount() int {
	st.mu.Lock()
	defer st.mu.Unlock()
	return len(st.sessions)
}

// generateSessionID creates a random session identifier.
func generateSessionID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("session-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("ses_%x", b)
}
