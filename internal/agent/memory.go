package agent

import (
	"sync"

	"test_task_goblin/internal/llm"
)

// Memory keeps a short, bounded conversation history per Telegram user,
// so a follow-up like "а подешевле есть?" or "удали ещё и такси" has
// context from the previous turn. It is intentionally minimal: in-process
// only (lost on restart — no requirement to persist it), and it only
// stores the user's message and the model's final reply, never the
// intermediate tool-calling exchange, to keep token usage down on every
// subsequent request.
type Memory struct {
	mu       sync.Mutex
	maxTurns int // number of (user, assistant) pairs kept per user
	byUser   map[int64][]llm.Message
}

// NewMemory builds a Memory keeping at most maxTurns of history per user.
func NewMemory(maxTurns int) *Memory {
	return &Memory{maxTurns: maxTurns, byUser: make(map[int64][]llm.Message)}
}

// History returns a copy of the stored turns for userID, oldest first.
func (m *Memory) History(userID int64) []llm.Message {
	m.mu.Lock()
	defer m.mu.Unlock()

	h := m.byUser[userID]
	out := make([]llm.Message, len(h))
	copy(out, h)
	return out
}

// Append records one finished turn, trimming the oldest if the history
// grew past maxTurns.
func (m *Memory) Append(userID int64, userMsg, assistantMsg llm.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()

	h := append(m.byUser[userID], userMsg, assistantMsg)
	if max := m.maxTurns * 2; len(h) > max {
		h = h[len(h)-max:]
	}
	m.byUser[userID] = h
}
