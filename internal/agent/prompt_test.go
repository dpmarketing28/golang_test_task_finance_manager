package agent

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"test_task_goblin/internal/domain"
)

func TestSystemPrompt_SubstitutesToday(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	got := systemPrompt(now, testMember)

	if !strings.Contains(got, "2026-10-16") {
		t.Errorf("rendered prompt does not contain today's date: %s", got)
	}
	if strings.Contains(got, "{{TODAY}}") {
		t.Errorf("rendered prompt still has the unsubstituted placeholder: %s", got)
	}
}

// This is what the code-review fix relies on: without the model knowing
// its own actor's telegram_id, it has no way to tell "my expenses" apart
// from "the whole family's" when results mix several owners.
func TestSystemPrompt_SubstitutesActorIdentity(t *testing.T) {
	got := systemPrompt(time.Now(), testAdmin)

	if !strings.Contains(got, strconv.FormatInt(testAdmin.TelegramID, 10)) {
		t.Errorf("rendered prompt does not contain the actor's telegram_id: %s", got)
	}
	if !strings.Contains(got, testAdmin.Name) {
		t.Errorf("rendered prompt does not contain the actor's name: %s", got)
	}
	if !strings.Contains(got, string(testAdmin.Role)) {
		t.Errorf("rendered prompt does not contain the actor's role: %s", got)
	}
	for _, placeholder := range []string{"{{USER_ID}}", "{{USER_NAME}}", "{{USER_ROLE}}"} {
		if strings.Contains(got, placeholder) {
			t.Errorf("rendered prompt still has unsubstituted placeholder %s", placeholder)
		}
	}
}

func TestSystemPrompt_MentionsEveryDomainCategory(t *testing.T) {
	// Guards against the prompt's hardcoded category list silently
	// drifting from domain.AllCategories, since this text (unlike the
	// tool schema) isn't generated from the enum.
	got := systemPrompt(time.Now(), testMember)

	for _, c := range domain.AllCategories {
		if !strings.Contains(got, string(c)) {
			t.Errorf("system prompt doesn't mention category %q", c)
		}
	}
}

func TestSystemPrompt_InstructsRussianReplies(t *testing.T) {
	got := systemPrompt(time.Now(), testMember)
	if !strings.Contains(got, "Russian") {
		t.Errorf("system prompt should instruct the model to reply in Russian: %s", got)
	}
}
