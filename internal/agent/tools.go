package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"test_task_goblin/internal/domain"
	"test_task_goblin/internal/llm"
	"test_task_goblin/internal/usecase"
)

const dateLayout = "2006-01-02"

// categoryEnum lists the fixed categories for the tool schemas, sourced
// from domain so the list is never duplicated.
func categoryEnum() []string {
	out := make([]string, len(domain.AllCategories))
	for i, c := range domain.AllCategories {
		out[i] = string(c)
	}
	return out
}

// userIDParam is the schema fragment shared by every tool that accepts an
// optional target family member. For a non-admin it's always ignored (the
// usecase layer forces their own id regardless); for an admin, omitting
// it means "the whole family" for list_expenses, or "myself" for
// get_last_expense/add_expense — see the system prompt for the exact rule.
var userIDParam = map[string]any{
	"type":        "integer",
	"description": "Telegram id of the family member to act on. Resolve a name to this via list_family_members first — never guess it.",
}

// toolSchemas are the functions the model is allowed to call. They map
// directly onto usecase.ExpenseService — the same access-controlled CRUD
// the REST API uses, so a member can't do anything through the bot that
// they couldn't do through the API, and vice versa.
var toolSchemas = []llm.Tool{
	{
		Type: "function",
		Function: llm.FunctionSchema{
			Name:        "add_expense",
			Description: "Record a new expense, by default for the current user.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"amount":   map[string]any{"type": "number", "description": "Positive amount spent"},
					"category": map[string]any{"type": "string", "enum": categoryEnum()},
					"spent_at": map[string]any{"type": "string", "description": "Date in YYYY-MM-DD format"},
					"comment":  map[string]any{"type": "string", "description": "Optional short note, e.g. the original word the user used"},
					"user_id":  map[string]any{"type": "integer", "description": "Only effective for admins: log this expense for another family member instead of yourself. Resolve a name to this via list_family_members first."},
				},
				"required": []string{"amount", "category", "spent_at"},
			},
		},
	},
	{
		Type: "function",
		Function: llm.FunctionSchema{
			Name:        "list_expenses",
			Description: "Read expenses, optionally filtered. Use this to answer questions like totals/periods, and to find the id of an expense the user refers to by description before updating or deleting it.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"category": map[string]any{"type": "string", "enum": categoryEnum()},
					"from":     map[string]any{"type": "string", "description": "YYYY-MM-DD, inclusive"},
					"to":       map[string]any{"type": "string", "description": "YYYY-MM-DD, inclusive"},
					"user_id":  userIDParam,
				},
			},
		},
	},
	{
		Type: "function",
		Function: llm.FunctionSchema{
			Name:        "get_last_expense",
			Description: "Get the most recently added expense, including its id. Use this for phrases like 'the last one'.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"user_id": userIDParam,
				},
			},
		},
	},
	{
		Type: "function",
		Function: llm.FunctionSchema{
			Name:        "update_expense",
			Description: "Change one or more fields of an existing expense. The id must come from list_expenses or get_last_expense, never guessed.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":       map[string]any{"type": "integer"},
					"amount":   map[string]any{"type": "number"},
					"category": map[string]any{"type": "string", "enum": categoryEnum()},
					"spent_at": map[string]any{"type": "string", "description": "YYYY-MM-DD"},
					"comment":  map[string]any{"type": "string"},
				},
				"required": []string{"id"},
			},
		},
	},
	{
		Type: "function",
		Function: llm.FunctionSchema{
			Name:        "delete_expense",
			Description: "Delete one expense by id. The id must come from list_expenses or get_last_expense, never guessed.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "integer"},
				},
				"required": []string{"id"},
			},
		},
	},
	{
		Type: "function",
		Function: llm.FunctionSchema{
			Name:        "list_family_members",
			Description: "List every family member with their telegram_id, name and role. Use this to resolve a name mentioned by the user (e.g. 'сын', 'дочь') to a telegram_id before passing it as user_id to another tool.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	},
}

// execute runs one tool call the model asked for and returns its result
// (or error) as a JSON string, to be fed back to the model as a "tool"
// message.
func (a *Agent) execute(ctx context.Context, actor domain.User, call llm.ToolCall) string {
	args := call.Function.Arguments

	switch call.Function.Name {
	case "add_expense":
		return a.toolAddExpense(ctx, actor, args)
	case "list_expenses":
		return a.toolListExpenses(ctx, actor, args)
	case "get_last_expense":
		return a.toolGetLastExpense(ctx, actor, args)
	case "update_expense":
		return a.toolUpdateExpense(ctx, actor, args)
	case "delete_expense":
		return a.toolDeleteExpense(ctx, actor, args)
	case "list_family_members":
		return a.toolListFamilyMembers(ctx)
	default:
		return toolError(fmt.Errorf("unknown tool %q", call.Function.Name))
	}
}

func toolResult(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(b)
}

func toolError(err error) string {
	return toolResult(map[string]string{"error": err.Error()})
}

// expenseView includes user_id so the model can tell whose expense is
// whose whenever a result mixes more than one family member (which
// happens for an admin who didn't scope the query to a specific user_id).
func expenseView(e domain.Expense) map[string]any {
	return map[string]any{
		"id":       e.ID,
		"user_id":  e.UserID,
		"amount":   e.Amount,
		"category": string(e.Category),
		"spent_at": e.SpentAt.Format(dateLayout),
		"comment":  e.Comment,
	}
}

func userView(u domain.User) map[string]any {
	return map[string]any{
		"telegram_id": u.TelegramID,
		"name":        u.Name,
		"role":        string(u.Role),
	}
}

type addExpenseArgs struct {
	Amount   float64 `json:"amount"`
	Category string  `json:"category"`
	SpentAt  string  `json:"spent_at"`
	Comment  string  `json:"comment,omitempty"`
	UserID   int64   `json:"user_id,omitempty"`
}

func (a *Agent) toolAddExpense(ctx context.Context, actor domain.User, raw string) string {
	var args addExpenseArgs
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return toolError(fmt.Errorf("invalid arguments: %w", err))
	}

	spentAt, err := time.Parse(dateLayout, args.SpentAt)
	if err != nil {
		return toolError(fmt.Errorf("invalid spent_at, expected YYYY-MM-DD: %w", err))
	}

	var comment *string
	if args.Comment != "" {
		comment = &args.Comment
	}

	e, err := a.expenses.Add(ctx, actor, usecase.AddExpenseInput{
		UserID:   args.UserID, // only honored for an admin actor; usecase forces self otherwise
		Amount:   args.Amount,
		Category: domain.Category(args.Category),
		SpentAt:  spentAt,
		Comment:  comment,
	})
	if err != nil {
		return toolError(err)
	}
	return toolResult(map[string]any{"ok": true, "expense": expenseView(*e)})
}

type listExpensesArgs struct {
	Category string `json:"category,omitempty"`
	From     string `json:"from,omitempty"`
	To       string `json:"to,omitempty"`
	UserID   *int64 `json:"user_id,omitempty"`
}

func (a *Agent) toolListExpenses(ctx context.Context, actor domain.User, raw string) string {
	var args listExpensesArgs
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &args); err != nil {
			return toolError(fmt.Errorf("invalid arguments: %w", err))
		}
	}

	filter := domain.ExpenseFilter{UserID: args.UserID} // only honored for an admin actor; usecase forces self otherwise
	if args.Category != "" {
		c := domain.Category(args.Category)
		filter.Category = &c
	}
	if args.From != "" {
		d, err := time.Parse(dateLayout, args.From)
		if err != nil {
			return toolError(fmt.Errorf("invalid from, expected YYYY-MM-DD: %w", err))
		}
		filter.From = &d
	}
	if args.To != "" {
		d, err := time.Parse(dateLayout, args.To)
		if err != nil {
			return toolError(fmt.Errorf("invalid to, expected YYYY-MM-DD: %w", err))
		}
		filter.To = &d
	}

	list, err := a.expenses.List(ctx, actor, filter)
	if err != nil {
		return toolError(err)
	}

	views := make([]map[string]any, 0, len(list))
	for _, e := range list {
		views = append(views, expenseView(e))
	}
	return toolResult(map[string]any{"expenses": views})
}

type lastExpenseArgs struct {
	UserID int64 `json:"user_id,omitempty"`
}

func (a *Agent) toolGetLastExpense(ctx context.Context, actor domain.User, raw string) string {
	var args lastExpenseArgs
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &args); err != nil {
			return toolError(fmt.Errorf("invalid arguments: %w", err))
		}
	}

	e, err := a.expenses.Last(ctx, actor, args.UserID) // 0 means "myself"; only honored for an admin actor otherwise
	if errors.Is(err, domain.ErrNotFound) {
		return toolResult(map[string]any{"expense": nil})
	}
	if err != nil {
		return toolError(err)
	}
	return toolResult(map[string]any{"expense": expenseView(*e)})
}

type updateExpenseArgs struct {
	ID       int64    `json:"id"`
	Amount   *float64 `json:"amount,omitempty"`
	Category *string  `json:"category,omitempty"`
	SpentAt  *string  `json:"spent_at,omitempty"`
	Comment  *string  `json:"comment,omitempty"`
}

func (a *Agent) toolUpdateExpense(ctx context.Context, actor domain.User, raw string) string {
	var args updateExpenseArgs
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return toolError(fmt.Errorf("invalid arguments: %w", err))
	}
	if args.ID == 0 {
		return toolError(fmt.Errorf("id is required"))
	}

	patch := usecase.UpdateExpenseInput{Amount: args.Amount, Comment: args.Comment}
	if args.Category != nil {
		c := domain.Category(*args.Category)
		patch.Category = &c
	}
	if args.SpentAt != nil {
		d, err := time.Parse(dateLayout, *args.SpentAt)
		if err != nil {
			return toolError(fmt.Errorf("invalid spent_at, expected YYYY-MM-DD: %w", err))
		}
		patch.SpentAt = &d
	}

	e, err := a.expenses.Update(ctx, actor, args.ID, patch)
	if err != nil {
		return toolError(err)
	}
	return toolResult(map[string]any{"ok": true, "expense": expenseView(*e)})
}

type deleteExpenseArgs struct {
	ID int64 `json:"id"`
}

func (a *Agent) toolDeleteExpense(ctx context.Context, actor domain.User, raw string) string {
	var args deleteExpenseArgs
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return toolError(fmt.Errorf("invalid arguments: %w", err))
	}
	if args.ID == 0 {
		return toolError(fmt.Errorf("id is required"))
	}

	if err := a.expenses.Delete(ctx, actor, args.ID); err != nil {
		return toolError(err)
	}
	return toolResult(map[string]any{"ok": true})
}

func (a *Agent) toolListFamilyMembers(ctx context.Context) string {
	users, err := a.users.List(ctx)
	if err != nil {
		return toolError(err)
	}

	views := make([]map[string]any, 0, len(users))
	for _, u := range users {
		views = append(views, userView(u))
	}
	return toolResult(map[string]any{"members": views})
}
