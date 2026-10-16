// Package telegram is the Telegram adapter: it only knows how to receive
// and send messages, register the sender as a family member, and route
// free-form text to the NLU agent. Business logic lives in
// internal/usecase; natural-language understanding lives in internal/agent.
package telegram

import (
	"context"
	"log"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"test_task_goblin/internal/agent"
	"test_task_goblin/internal/domain"
	"test_task_goblin/internal/usecase"
)

// agentTimeout bounds one message's round trip through the LLM (which
// may call several tools), so a stuck provider can't hang the bot.
const agentTimeout = 45 * time.Second

// Bot wraps the Telegram Bot API client and dispatches incoming updates.
type Bot struct {
	api   *tgbotapi.BotAPI
	users *usecase.UserService
	agent *agent.Agent
}

// New creates a Bot authenticated with the given token.
func New(token string, users *usecase.UserService, agent *agent.Agent) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}

	return &Bot{api: api, users: users, agent: agent}, nil
}

// Run starts long polling and blocks, handling updates until Stop is
// called.
func (b *Bot) Run() error {
	log.Printf("telegram: authorized as @%s", b.api.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		// Each message can take up to agentTimeout going through the LLM
		// (possibly several tool-call round trips). Handling updates one
		// at a time here would make one family member's slow message
		// block everyone else's; dispatching each to its own goroutine
		// lets family members be served concurrently. The one corner
		// case this doesn't preserve is strict ordering between two
		// messages sent in quick succession by the *same* person — an
		// acceptable trade-off for a low-traffic family bot.
		msg := update.Message
		go b.handleMessage(msg)
	}

	return nil
}

// Stop ends long polling so Run can return during graceful shutdown.
func (b *Bot) Stop() {
	b.api.StopReceivingUpdates()
}

// handleMessage routes a single incoming message to the right handler.
// Every message first registers (or refreshes) the sender as a family
// member, since that's the only place Telegram users become known to us.
//
// /start is handled right here and deliberately never reaches the agent:
// it's a fixed greeting, not something that should cost an LLM call or
// show up in the model's conversation history.
func (b *Bot) handleMessage(msg *tgbotapi.Message) {
	name := msg.From.FirstName
	if msg.From.UserName != "" {
		name = msg.From.UserName
	}

	user, err := b.users.EnsureRegistered(context.Background(), msg.From.ID, name)
	if err != nil {
		log.Printf("telegram: register user %d failed: %v", msg.From.ID, err)
		b.reply(msg.Chat.ID, "Произошла ошибка, попробуйте позже.")
		return
	}

	switch msg.Command() {
	case "start":
		greeting := "Привет! Я бот учёта семейных расходов 👋\nПросто напиши трату: «кофе 350», «удали такси», «сколько потратил на еду в этом месяце» — я разберусь сам."
		if user.IsAdmin() {
			greeting += "\nТы глава семьи — тебе доступны траты всех членов семьи."
		}
		b.reply(msg.Chat.ID, greeting)
	default:
		b.handleFreeText(*user, msg)
	}
}

// handleFreeText is everything that isn't /start: it goes to the NLU
// agent, which decides which CRUD tool(s) to call and what to answer.
func (b *Bot) handleFreeText(user domain.User, msg *tgbotapi.Message) {
	if msg.Text == "" {
		b.reply(msg.Chat.ID, "Напиши, пожалуйста, текстом — так я понимаю траты.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), agentTimeout)
	defer cancel()

	reply := b.agent.Handle(ctx, user, msg.Text)
	b.reply(msg.Chat.ID, reply)
}

// reply sends a plain text message to the given chat.
func (b *Bot) reply(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("telegram: send message failed: %v", err)
	}
}
