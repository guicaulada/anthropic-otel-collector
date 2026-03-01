package anthropicreceiver

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionTracker_NewSession(t *testing.T) {
	st := newSessionTracker(30 * time.Minute)
	defer st.Stop()

	ccCtx := ClaudeCodeContext{
		IsClaudeCode:      true,
		WorkingDir:        "/home/user/project",
		ProjectName:       "project",
		ConversationDepth: 0,
	}

	sc := st.TrackRequest("abcd", ccCtx, 0.01, 100, 20, time.Second)

	assert.NotEmpty(t, sc.SessionID)
	assert.Equal(t, "/home/user/project", sc.ProjectPath)
	assert.Equal(t, "project", sc.ProjectName)
	assert.Equal(t, 1, sc.RequestNumber)
	assert.True(t, sc.IsNewSession)
}

func TestSessionTracker_ContinuesSession(t *testing.T) {
	st := newSessionTracker(30 * time.Minute)
	defer st.Stop()

	ccCtx := ClaudeCodeContext{
		IsClaudeCode:      true,
		WorkingDir:        "/home/user/project",
		ProjectName:       "project",
		ConversationDepth: 1,
	}

	sc1 := st.TrackRequest("abcd", ccCtx, 0.01, 100, 20, time.Second)
	assert.True(t, sc1.IsNewSession)

	ccCtx.ConversationDepth = 2
	sc2 := st.TrackRequest("abcd", ccCtx, 0.02, 200, 40, 2*time.Second)

	assert.Equal(t, sc1.SessionID, sc2.SessionID)
	assert.Equal(t, 2, sc2.RequestNumber)
	assert.False(t, sc2.IsNewSession)
}

func TestSessionTracker_DifferentProjects(t *testing.T) {
	st := newSessionTracker(30 * time.Minute)
	defer st.Stop()

	ctx1 := ClaudeCodeContext{
		IsClaudeCode: true,
		WorkingDir:   "/home/user/project-a",
		ProjectName:  "project-a",
	}
	ctx2 := ClaudeCodeContext{
		IsClaudeCode: true,
		WorkingDir:   "/home/user/project-b",
		ProjectName:  "project-b",
	}

	sc1 := st.TrackRequest("abcd", ctx1, 0.01, 100, 20, time.Second)
	sc2 := st.TrackRequest("abcd", ctx2, 0.01, 100, 20, time.Second)

	assert.NotEqual(t, sc1.SessionID, sc2.SessionID)
	assert.Equal(t, 2, st.activeSessionCount())
}

func TestSessionTracker_DifferentAPIKeys(t *testing.T) {
	st := newSessionTracker(30 * time.Minute)
	defer st.Stop()

	ccCtx := ClaudeCodeContext{
		IsClaudeCode: true,
		WorkingDir:   "/home/user/project",
		ProjectName:  "project",
	}

	sc1 := st.TrackRequest("key1", ccCtx, 0.01, 100, 20, time.Second)
	sc2 := st.TrackRequest("key2", ccCtx, 0.01, 100, 20, time.Second)

	assert.NotEqual(t, sc1.SessionID, sc2.SessionID)
}

func TestSessionTracker_Timeout(t *testing.T) {
	// Use a very short timeout for testing
	st := &sessionTracker{
		sessions: make(map[sessionKey]*sessionState),
		timeout:  50 * time.Millisecond,
		done:     make(chan struct{}),
	}
	defer st.Stop()

	ccCtx := ClaudeCodeContext{
		IsClaudeCode:      true,
		WorkingDir:        "/home/user/project",
		ProjectName:       "project",
		ConversationDepth: 1,
	}

	sc1 := st.TrackRequest("abcd", ccCtx, 0.01, 100, 20, time.Second)

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	sc2 := st.TrackRequest("abcd", ccCtx, 0.01, 100, 20, time.Second)

	assert.NotEqual(t, sc1.SessionID, sc2.SessionID, "should create new session after timeout")
	assert.True(t, sc2.IsNewSession)
	assert.Equal(t, 1, sc2.RequestNumber)
}

func TestSessionTracker_ConversationReset(t *testing.T) {
	st := newSessionTracker(30 * time.Minute)
	defer st.Stop()

	ccCtx := ClaudeCodeContext{
		IsClaudeCode:      true,
		WorkingDir:        "/home/user/project",
		ProjectName:       "project",
		ConversationDepth: 10,
	}

	sc1 := st.TrackRequest("abcd", ccCtx, 0.01, 100, 20, time.Second)

	// Depth increases further
	ccCtx.ConversationDepth = 15
	sc2 := st.TrackRequest("abcd", ccCtx, 0.01, 100, 20, time.Second)
	assert.Equal(t, sc1.SessionID, sc2.SessionID, "same session with increasing depth")

	// Depth drops significantly (below peak/2 = 7)
	ccCtx.ConversationDepth = 2
	sc3 := st.TrackRequest("abcd", ccCtx, 0.01, 100, 20, time.Second)
	assert.NotEqual(t, sc2.SessionID, sc3.SessionID, "new session on conversation reset")
	assert.True(t, sc3.IsNewSession)
}

func TestSessionTracker_ConversationReset_SmallPeak(t *testing.T) {
	st := newSessionTracker(30 * time.Minute)
	defer st.Stop()

	ccCtx := ClaudeCodeContext{
		IsClaudeCode:      true,
		WorkingDir:        "/home/user/project",
		ProjectName:       "project",
		ConversationDepth: 2,
	}

	sc1 := st.TrackRequest("abcd", ccCtx, 0.01, 100, 20, time.Second)

	// Drop to 0 but peak is only 2 — should NOT reset
	ccCtx.ConversationDepth = 0
	sc2 := st.TrackRequest("abcd", ccCtx, 0.01, 100, 20, time.Second)
	assert.Equal(t, sc1.SessionID, sc2.SessionID, "should not reset for small peak depth")
}

func TestSessionTracker_Cleanup(t *testing.T) {
	st := &sessionTracker{
		sessions: make(map[sessionKey]*sessionState),
		timeout:  50 * time.Millisecond,
		done:     make(chan struct{}),
	}
	defer st.Stop()

	ccCtx := ClaudeCodeContext{
		IsClaudeCode: true,
		WorkingDir:   "/home/user/project",
		ProjectName:  "project",
	}

	st.TrackRequest("abcd", ccCtx, 0.01, 100, 20, time.Second)
	assert.Equal(t, 1, st.activeSessionCount())

	// Wait for timeout and manually trigger cleanup
	time.Sleep(60 * time.Millisecond)
	st.cleanup()

	assert.Equal(t, 0, st.activeSessionCount())
}

func TestSessionTracker_ConcurrentAccess(t *testing.T) {
	st := newSessionTracker(30 * time.Minute)
	defer st.Stop()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ccCtx := ClaudeCodeContext{
				IsClaudeCode:      true,
				WorkingDir:        "/home/user/project",
				ProjectName:       "project",
				ConversationDepth: i,
			}
			sc := st.TrackRequest("abcd", ccCtx, 0.01, 100, 20, time.Second)
			assert.NotEmpty(t, sc.SessionID)
		}(i)
	}
	wg.Wait()

	// All concurrent requests to the same key should have the same session
	assert.Equal(t, 1, st.activeSessionCount())
}

func TestIsConversationReset(t *testing.T) {
	tests := []struct {
		name     string
		current  int
		peak     int
		expected bool
	}{
		{"peak is 0", 0, 0, false},
		{"peak is 1", 0, 1, false},
		{"peak is 2", 0, 2, false},
		{"peak is 3, current is 0", 0, 3, true},
		{"peak is 10, current is 4", 4, 10, true},
		{"peak is 10, current is 5", 5, 10, false},
		{"peak is 10, current is 6", 6, 10, false},
		{"peak is 20, current is 9", 9, 20, true},
		{"peak is 20, current is 10", 10, 20, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isConversationReset(tt.current, tt.peak))
		})
	}
}

func TestGenerateSessionID(t *testing.T) {
	id1 := generateSessionID()
	id2 := generateSessionID()

	require.NotEmpty(t, id1)
	require.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2, "session IDs should be unique")
	assert.Contains(t, id1, "ses_", "session ID should have ses_ prefix")
}

func TestSessionTracker_AccumulatesState(t *testing.T) {
	st := newSessionTracker(30 * time.Minute)
	defer st.Stop()

	ccCtx := ClaudeCodeContext{
		IsClaudeCode:      true,
		WorkingDir:        "/home/user/project",
		ProjectName:       "project",
		ConversationDepth: 1,
	}

	st.TrackRequest("abcd", ccCtx, 0.01, 100, 20, time.Second)
	ccCtx.ConversationDepth = 2
	st.TrackRequest("abcd", ccCtx, 0.02, 200, 40, 2*time.Second)
	ccCtx.ConversationDepth = 3
	sc := st.TrackRequest("abcd", ccCtx, 0.03, 300, 60, 3*time.Second)

	assert.Equal(t, 3, sc.RequestNumber)

	// Verify internal state
	st.mu.Lock()
	key := sessionKey{apiKeyHash: "abcd", projectPath: "/home/user/project"}
	state := st.sessions[key]
	st.mu.Unlock()

	assert.InDelta(t, 0.06, state.cumulativeCost, 0.001)
	assert.Equal(t, 600, state.cumulativeInput)
	assert.Equal(t, 120, state.cumulativeOutput)
	assert.Equal(t, 6*time.Second, state.cumulativeDuration)
	assert.Equal(t, 3, state.peakConversation)
}
