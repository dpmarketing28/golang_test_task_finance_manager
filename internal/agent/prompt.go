package agent

import (
	_ "embed"
	"strconv"
	"strings"
	"time"

	"test_task_goblin/internal/domain"
)

// The system prompt is written in English deliberately: it's never shown
// to the user (only the model reads it), and English tokenizes cheaper
// than Russian for most tokenizers — this is pure instruction text, so
// there's no reason to pay more for it. The model is explicitly told to
// reply to the user in Russian regardless.
//
//go:embed prompts/system.txt
var systemPromptTemplate string

// systemPrompt renders the prompt with today's date and the acting
// family member's own identity. The model needs its own telegram_id to
// tell "my expenses" apart from "the whole family's expenses" when it's
// talking to the admin — without it, an admin asking "сколько я
// потратил" has no way to be scoped to just themselves.
func systemPrompt(now time.Time, actor domain.User) string {
	s := systemPromptTemplate
	s = strings.ReplaceAll(s, "{{TODAY}}", now.Format("2006-01-02"))
	s = strings.ReplaceAll(s, "{{USER_ID}}", strconv.FormatInt(actor.TelegramID, 10))
	s = strings.ReplaceAll(s, "{{USER_NAME}}", actor.Name)
	s = strings.ReplaceAll(s, "{{USER_ROLE}}", string(actor.Role))
	return s
}
