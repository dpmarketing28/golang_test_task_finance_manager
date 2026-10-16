package agent

import (
	"testing"

	"test_task_goblin/internal/llm"
)

func TestMemory_EmptyHistoryForUnknownUser(t *testing.T) {
	m := NewMemory(3)
	if h := m.History(123); len(h) != 0 {
		t.Errorf("history for unknown user = %+v, want empty", h)
	}
}

func TestMemory_AppendAndHistoryOrder(t *testing.T) {
	m := NewMemory(3)
	m.Append(123, llm.Message{Role: "user", Content: "кофе 350"}, llm.Message{Role: "assistant", Content: "добавил"})
	m.Append(123, llm.Message{Role: "user", Content: "и такси 1200"}, llm.Message{Role: "assistant", Content: "добавил"})

	h := m.History(123)
	if len(h) != 4 {
		t.Fatalf("history length = %d, want 4 (2 turns x 2 messages)", len(h))
	}
	if h[0].Content != "кофе 350" || h[2].Content != "и такси 1200" {
		t.Errorf("history order wrong: %+v", h)
	}
}

func TestMemory_TrimsOldestPastMaxTurns(t *testing.T) {
	m := NewMemory(2) // keep at most 2 turns = 4 messages

	for i := 0; i < 5; i++ {
		m.Append(123,
			llm.Message{Role: "user", Content: itoa(int64(i))},
			llm.Message{Role: "assistant", Content: "ok" + itoa(int64(i))},
		)
	}

	h := m.History(123)
	if len(h) != 4 {
		t.Fatalf("history length = %d, want 4 after trimming", len(h))
	}
	// Only the last two turns (i=3 and i=4) should remain.
	if h[0].Content != "3" || h[2].Content != "4" {
		t.Errorf("expected the two most recent turns to survive trimming, got %+v", h)
	}
}

func TestMemory_PerUserIsolation(t *testing.T) {
	m := NewMemory(5)
	m.Append(111, llm.Message{Role: "user", Content: "admin message"}, llm.Message{Role: "assistant", Content: "ok"})

	if h := m.History(222); len(h) != 0 {
		t.Errorf("user 222's history = %+v, want empty (must not see user 111's history)", h)
	}
}

func TestMemory_HistoryReturnsACopy(t *testing.T) {
	m := NewMemory(5)
	m.Append(123, llm.Message{Role: "user", Content: "a"}, llm.Message{Role: "assistant", Content: "b"})

	h := m.History(123)
	h[0].Content = "mutated"

	if got := m.History(123)[0].Content; got != "a" {
		t.Errorf("mutating the returned slice affected stored history: got %q, want %q", got, "a")
	}
}
