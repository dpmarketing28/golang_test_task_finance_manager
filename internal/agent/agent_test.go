package agent

import (
	"context"
	"sync"
	"testing"

	"test_task_goblin/internal/llm"
	"test_task_goblin/internal/usecase"
)

// fakeChatCompleter replays a scripted sequence of assistant responses,
// recording the messages it was called with so tests can assert on the
// exact conversation the loop built (history included, tool results fed
// back, etc). It never touches the network.
type fakeChatCompleter struct {
	mu       sync.Mutex
	calls    [][]llm.Message
	scripted []llm.Message
	err      error // if set, every call fails with this error
	// exhaustedErr, if set, is returned once scripted responses run out
	// instead of panicking — used to simulate "the next call fails too"
	// without needing to know its exact response in advance.
	exhaustedErr error
}

func (f *fakeChatCompleter) CreateChatCompletion(_ context.Context, req llm.ChatRequest) (llm.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	msgs := make([]llm.Message, len(req.Messages))
	copy(msgs, req.Messages)
	f.calls = append(f.calls, msgs)

	if f.err != nil {
		return llm.Message{}, f.err
	}
	if len(f.scripted) == 0 {
		if f.exhaustedErr != nil {
			return llm.Message{}, f.exhaustedErr
		}
		panic("fakeChatCompleter: ran out of scripted responses")
	}
	resp := f.scripted[0]
	f.scripted = f.scripted[1:]
	return resp, nil
}

func (f *fakeChatCompleter) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func toolCallMsg(id, name, args string) llm.Message {
	call := llm.ToolCall{ID: id, Type: "function"}
	call.Function.Name = name
	call.Function.Arguments = args
	return llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{call}}
}

func newAgentWithFakeLLM(scripted ...llm.Message) (*Agent, *fakeChatCompleter, *fakeExpenseRepo) {
	expenseRepo := newFakeExpenseRepo()
	userRepo := newFakeUserRepo(testMember, testAdmin, testOther)
	fake := &fakeChatCompleter{scripted: scripted}
	a := New(fake, usecase.NewExpenseService(expenseRepo), usecase.NewUserService(userRepo, 0), NewMemory(6))
	return a, fake, expenseRepo
}

func TestHandle_ImmediateAnswerNoTools(t *testing.T) {
	a, fake, _ := newAgentWithFakeLLM(llm.Message{Role: "assistant", Content: "Привет! Чем могу помочь?"})

	got := a.Handle(context.Background(), testMember, "привет")

	if got != "Привет! Чем могу помочь?" {
		t.Errorf("reply = %q", got)
	}
	if fake.callCount() != 1 {
		t.Errorf("llm calls = %d, want 1", fake.callCount())
	}
}

func TestHandle_ExecutesToolThenAnswers(t *testing.T) {
	a, fake, repo := newAgentWithFakeLLM(
		toolCallMsg("call1", "add_expense", `{"amount":350,"category":"еда","spent_at":"2026-10-16","comment":"кофе"}`),
		llm.Message{Role: "assistant", Content: "Добавил кофе за 350."},
	)

	got := a.Handle(context.Background(), testMember, "кофе 350")

	if got != "Добавил кофе за 350." {
		t.Errorf("reply = %q", got)
	}
	if len(repo.expenses) != 1 {
		t.Fatalf("repo has %d expenses, want 1 (tool call should have run)", len(repo.expenses))
	}
	if fake.callCount() != 2 {
		t.Errorf("llm calls = %d, want 2 (one for the tool call, one for the final answer)", fake.callCount())
	}

	// The second call must include a "tool" message answering call1.
	secondCallMsgs := fake.calls[1]
	foundToolResult := false
	for _, m := range secondCallMsgs {
		if m.Role == "tool" && m.ToolCallID == "call1" {
			foundToolResult = true
		}
	}
	if !foundToolResult {
		t.Errorf("second llm call did not include the tool result for call1: %+v", secondCallMsgs)
	}
}

func TestHandle_MultipleToolCallsInOneRound(t *testing.T) {
	round1 := llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{
		mustToolCall("call1", "add_expense", `{"amount":100,"category":"еда","spent_at":"2026-10-16"}`),
		mustToolCall("call2", "add_expense", `{"amount":200,"category":"транспорт","spent_at":"2026-10-16"}`),
	}}
	a, fake, repo := newAgentWithFakeLLM(round1, llm.Message{Role: "assistant", Content: "Добавил обе траты."})

	got := a.Handle(context.Background(), testMember, "добавь две траты")

	if got != "Добавил обе траты." {
		t.Errorf("reply = %q", got)
	}
	if len(repo.expenses) != 2 {
		t.Errorf("repo has %d expenses, want 2 (both tool calls should have run)", len(repo.expenses))
	}

	secondCallMsgs := fake.calls[1]
	gotIDs := map[string]bool{}
	for _, m := range secondCallMsgs {
		if m.Role == "tool" {
			gotIDs[m.ToolCallID] = true
		}
	}
	if !gotIDs["call1"] || !gotIDs["call2"] {
		t.Errorf("expected tool results for both call1 and call2, got %+v", secondCallMsgs)
	}
}

// When the model keeps asking for tools without ever giving a final
// answer, the loop must stop after maxToolRounds — but instead of just
// giving up (which would discard whatever the model already did, like
// expenses it already added), it makes one extra call with tools
// disabled to force a real summary of what happened so far.
func TestHandle_MaxToolRounds_ForcesAFinalSummaryInstead(t *testing.T) {
	scripted := make([]llm.Message, 0, maxToolRounds+1)
	for i := 0; i < maxToolRounds; i++ {
		scripted = append(scripted, toolCallMsg(itoa(int64(i)), "get_last_expense", "{}"))
	}
	scripted = append(scripted, llm.Message{Role: "assistant", Content: "Добавил всё, что успел разобрать."})

	a, fake, _ := newAgentWithFakeLLM(scripted...)

	got := a.Handle(context.Background(), testMember, "запутанный запрос")

	if fake.callCount() != maxToolRounds+1 {
		t.Errorf("llm calls = %d, want exactly %d (the cap, plus one forced final call)", fake.callCount(), maxToolRounds+1)
	}
	if got != "Добавил всё, что успел разобрать." {
		t.Errorf("reply = %q, want the model's forced final summary, not a generic fallback", got)
	}

	// The forced-but-real summary is a genuine model answer, so it must
	// be persisted like any other successful turn.
	history := a.memory.History(testMember.TelegramID)
	if len(history) != 2 {
		t.Errorf("history = %+v, want the turn to be persisted since a real answer was produced", history)
	}
}

// If even the forced final call fails (provider down, empty reply), the
// loop must fall back to a local message rather than panic or hang — and
// that fallback must NOT be persisted to memory (see the "not just
// something the model said" rule on Handle).
func TestHandle_MaxToolRounds_ForcedFinalAnswerAlsoFails(t *testing.T) {
	scripted := make([]llm.Message, 0, maxToolRounds)
	for i := 0; i < maxToolRounds; i++ {
		scripted = append(scripted, toolCallMsg(itoa(int64(i)), "get_last_expense", "{}"))
	}
	// The forced (maxToolRounds+1)-th call fails too — standing in for
	// "the provider errored on that call as well".
	fake := &fakeChatCompleter{scripted: scripted, exhaustedErr: errBoom}
	repo := newFakeExpenseRepo()
	a := New(fake, usecase.NewExpenseService(repo), usecase.NewUserService(newFakeUserRepo(testMember), 0), NewMemory(6))

	got := a.Handle(context.Background(), testMember, "запутанный запрос")

	if got == "" {
		t.Errorf("expected a non-empty fallback message")
	}
	if history := a.memory.History(testMember.TelegramID); len(history) != 0 {
		t.Errorf("history = %+v, want nothing persisted for a fallback that isn't a real answer", history)
	}
}

func TestHandle_LLMErrorReturnsApologyNotPanic(t *testing.T) {
	repo := newFakeExpenseRepo()
	svc := usecase.NewExpenseService(repo)
	fake := &fakeChatCompleter{err: errBoom}
	a := New(fake, svc, usecase.NewUserService(newFakeUserRepo(testMember), 0), NewMemory(6))

	got := a.Handle(context.Background(), testMember, "кофе 350")

	if got == "" {
		t.Errorf("expected a non-empty apology when the LLM call fails")
	}
	if len(repo.expenses) != 0 {
		t.Errorf("repo has %d expenses, want 0 (no tool should have run)", len(repo.expenses))
	}
}

func TestHandle_HistoryFeedsIntoNextCall(t *testing.T) {
	repo := newFakeExpenseRepo()
	svc := usecase.NewExpenseService(repo)
	fake := &fakeChatCompleter{scripted: []llm.Message{
		{Role: "assistant", Content: "Добавил кофе."},
		{Role: "assistant", Content: "Добавил такси."},
	}}
	a := New(fake, svc, usecase.NewUserService(newFakeUserRepo(testMember), 0), NewMemory(6))

	a.Handle(context.Background(), testMember, "кофе 350")
	a.Handle(context.Background(), testMember, "такси 1200")

	secondCallMsgs := fake.calls[1]
	var sawFirstUserMsg, sawFirstAssistantMsg bool
	for _, m := range secondCallMsgs {
		if m.Role == "user" && m.Content == "кофе 350" {
			sawFirstUserMsg = true
		}
		if m.Role == "assistant" && m.Content == "Добавил кофе." {
			sawFirstAssistantMsg = true
		}
	}
	if !sawFirstUserMsg || !sawFirstAssistantMsg {
		t.Errorf("second call should include the first turn from memory, got %+v", secondCallMsgs)
	}
}

func TestHandle_ToolScratchIsNotPersistedToMemory(t *testing.T) {
	a, _, _ := newAgentWithFakeLLM(
		toolCallMsg("call1", "add_expense", `{"amount":350,"category":"еда","spent_at":"2026-10-16"}`),
		llm.Message{Role: "assistant", Content: "Добавил."},
	)

	a.Handle(context.Background(), testMember, "кофе 350")

	history := a.memory.History(testMember.TelegramID)
	if len(history) != 2 {
		t.Fatalf("history length = %d, want 2 (just the user message and the final answer)", len(history))
	}
	for _, m := range history {
		if m.Role == "tool" || len(m.ToolCalls) > 0 {
			t.Errorf("tool-calling scratch work leaked into persisted memory: %+v", m)
		}
	}
}

func mustToolCall(id, name, args string) llm.ToolCall {
	c := llm.ToolCall{ID: id, Type: "function"}
	c.Function.Name = name
	c.Function.Arguments = args
	return c
}

var errBoom = &fakeErr{"boom"}

type fakeErr struct{ msg string }

func (e *fakeErr) Error() string { return e.msg }
