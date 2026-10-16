// Package telegram is the Telegram adapter: it only knows how to receive
// and send messages, and to register the sender as a family member.
// Business logic lives in internal/usecase; NLU/LLM parsing of free-form
// text will be wired into the default case below later.
package telegram

import (
	"context"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"test_task_goblin/internal/usecase"
)

// Bot wraps the Telegram Bot API client and dispatches incoming updates.
type Bot struct {
	api   *tgbotapi.BotAPI
	users *usecase.UserService
}

// New creates a Bot authenticated with the given token.
func New(token string, users *usecase.UserService) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}

	return &Bot{api: api, users: users}, nil
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

		b.handleMessage(update.Message)
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
		greeting := "Привет! Я бот учёта семейных расходов 👋"
		if user.IsAdmin() {
			greeting += "\nТы глава семьи — тебе доступны траты всех членов семьи."
		}
		b.reply(msg.Chat.ID, greeting)
	default:
		// TODO: route free-form text to the NLU/agent layer (add/list/
		// update/delete expenses) once it is implemented.
	}
}

// reply sends a plain text message to the given chat.
func (b *Bot) reply(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("telegram: send message failed: %v", err)
	}
}
