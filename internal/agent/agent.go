// Package agent turns a family member's free-form Telegram message into
// expense CRUD operations, using an LLM with tool calling. It is the only
// package that knows the system prompt and the tool schemas; everything
// it actually does to the data goes through usecase.ExpenseService, so
// the same access rules as the REST API apply automatically (a member
// can't touch anyone else's expenses through the bot either).
package agent

import (
	"context"
	"log"
	"time"

	"test_task_goblin/internal/domain"
	"test_task_goblin/internal/llm"
	"test_task_goblin/internal/usecase"
)

// maxToolRounds caps how many times the model may call tools for a
// single message, so a confused model can't loop forever.
const maxToolRounds = 4

// chatCompleter is the subset of llm.Client this package needs. It's
// declared here, at the point of use, rather than depending on the
// concrete *llm.Client — the same "accept interfaces" pattern as the
// repository ports in internal/usecase. It lets tests drive the
// tool-calling loop with a scripted fake instead of a real network call.
type chatCompleter interface {
	CreateChatCompletion(ctx context.Context, req llm.ChatRequest) (llm.Message, error)
}

// Agent handles one free-form message at a time.
type Agent struct {
	llm      chatCompleter
	expenses *usecase.ExpenseService
	users    *usecase.UserService
	memory   *Memory
}

func New(client chatCompleter, expenses *usecase.ExpenseService, users *usecase.UserService, memory *Memory) *Agent {
	return &Agent{llm: client, expenses: expenses, users: users, memory: memory}
}

// Handle turns text from actor into tool calls and a reply. It never
// returns an error to the caller — on any failure it logs the detail and
// returns a short apology in Russian, since the caller is a chat bot
// that just needs something to say back.
//
// A locally-generated fallback (LLM error, empty reply, round cap) is
// never saved to Memory: it isn't something the model actually said, and
// feeding it back as the model's own prior turn next time would have it
// react to an apology that has nothing to do with the next request.
func (a *Agent) Handle(ctx context.Context, actor domain.User, text string) string {
	sys := llm.Message{Role: "system", Content: systemPrompt(time.Now(), actor)}
	userMsg := llm.Message{Role: "user", Content: text}

	messages := append([]llm.Message{sys}, a.memory.History(actor.TelegramID)...)
	messages = append(messages, userMsg)

	finalText, ok := a.loop(ctx, actor, messages)
	if ok {
		a.memory.Append(actor.TelegramID, userMsg, llm.Message{Role: "assistant", Content: finalText})
	}
	return finalText
}

// loop drives the tool-calling exchange: ask the model, run whatever
// tools it asked for, feed the results back, repeat until it answers in
// plain text or the round cap is hit. The intermediate tool-calling
// messages built up here are local to this call and are never saved to
// Memory regardless of outcome — only Handle's final answer is, to keep
// future requests cheap.
//
// The returned bool is false whenever the text is a local fallback
// rather than something the model actually said.
func (a *Agent) loop(ctx context.Context, actor domain.User, messages []llm.Message) (string, bool) {
	for round := 0; round < maxToolRounds; round++ {
		reply, err := a.llm.CreateChatCompletion(ctx, llm.ChatRequest{Messages: messages, Tools: toolSchemas})
		if err != nil {
			log.Printf("agent: llm call failed: %v", err)
			return "Не получилось обработать сообщение, попробуй ещё раз.", false
		}

		messages = append(messages, reply)

		if len(reply.ToolCalls) == 0 {
			if reply.Content == "" {
				return "Не смог разобраться с запросом, уточни, пожалуйста.", false
			}
			return reply.Content, true
		}

		for _, call := range reply.ToolCalls {
			result := a.execute(ctx, actor, call)
			messages = append(messages, llm.Message{Role: "tool", ToolCallID: call.ID, Content: result})
		}
	}

	// The model never stopped asking for tools. By this point it may
	// well have already added/changed/deleted real data in earlier
	// rounds — telling the user "didn't work, try again" here would be
	// both wrong and an invitation to retry and duplicate what already
	// succeeded. So instead of giving up, force one last answer with
	// tools disabled: the model must summarize from what it already did
	// in this same conversation, not start over.
	log.Printf("agent: hit max tool rounds (%d) for user %d, forcing a final answer", maxToolRounds, actor.TelegramID)

	reply, err := a.llm.CreateChatCompletion(ctx, llm.ChatRequest{Messages: messages})
	if err != nil || reply.Content == "" {
		log.Printf("agent: forced final answer also failed: %v", err)
		return "Частично выполнил запрос, но не смог подвести итог — посмотри свои траты, чтобы проверить результат.", false
	}
	return reply.Content, true
}
