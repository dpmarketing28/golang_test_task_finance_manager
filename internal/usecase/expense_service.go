package usecase

import (
	"context"
	"fmt"
	"time"

	"test_task_goblin/internal/domain"
)

// ExpenseService implements CRUD over expenses with the access rule from
// the task: a regular member only ever sees and touches their own
// expenses; the family head (admin) sees and touches everyone's.
type ExpenseService struct {
	expenses ExpenseRepository
}

func NewExpenseService(expenses ExpenseRepository) *ExpenseService {
	return &ExpenseService{expenses: expenses}
}

// AddExpenseInput is what's needed to record a new expense. UserID is the
// intended owner; it is only honored for an admin actor acting on behalf
// of someone else, members always end up owning their own expense.
type AddExpenseInput struct {
	UserID   int64
	Amount   float64
	Category domain.Category
	SpentAt  time.Time
	Comment  *string
}

// UpdateExpenseInput is a partial update: nil fields are left unchanged.
type UpdateExpenseInput struct {
	Amount   *float64
	Category *domain.Category
	SpentAt  *time.Time
	Comment  *string
}

func (s *ExpenseService) Add(ctx context.Context, actor domain.User, in AddExpenseInput) (*domain.Expense, error) {
	if !in.Category.Valid() {
		return nil, fmt.Errorf("%w: unknown category %q", domain.ErrInvalidInput, in.Category)
	}
	if in.Amount <= 0 {
		return nil, fmt.Errorf("%w: amount must be positive", domain.ErrInvalidInput)
	}

	ownerID := actor.TelegramID
	if actor.IsAdmin() && in.UserID != 0 {
		ownerID = in.UserID // admin may log an expense for another family member
	}

	e := &domain.Expense{
		UserID:   ownerID,
		Amount:   in.Amount,
		Category: in.Category,
		SpentAt:  in.SpentAt,
		Comment:  in.Comment,
	}
	return s.expenses.Create(ctx, e)
}

// List returns expenses visible to actor. A non-admin's filter.UserID is
// always overridden to themselves, regardless of what was requested.
func (s *ExpenseService) List(ctx context.Context, actor domain.User, filter domain.ExpenseFilter) ([]domain.Expense, error) {
	if !actor.IsAdmin() {
		uid := actor.TelegramID
		filter.UserID = &uid
	}
	return s.expenses.List(ctx, filter)
}

func (s *ExpenseService) Get(ctx context.Context, actor domain.User, id int64) (*domain.Expense, error) {
	e, err := s.expenses.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !actor.IsAdmin() && e.UserID != actor.TelegramID {
		return nil, domain.ErrForbidden
	}
	return e, nil
}

func (s *ExpenseService) Update(ctx context.Context, actor domain.User, id int64, patch UpdateExpenseInput) (*domain.Expense, error) {
	e, err := s.expenses.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !actor.IsAdmin() && e.UserID != actor.TelegramID {
		return nil, domain.ErrForbidden
	}

	if patch.Amount != nil {
		if *patch.Amount <= 0 {
			return nil, fmt.Errorf("%w: amount must be positive", domain.ErrInvalidInput)
		}
		e.Amount = *patch.Amount
	}
	if patch.Category != nil {
		if !patch.Category.Valid() {
			return nil, fmt.Errorf("%w: unknown category %q", domain.ErrInvalidInput, *patch.Category)
		}
		e.Category = *patch.Category
	}
	if patch.SpentAt != nil {
		e.SpentAt = *patch.SpentAt
	}
	if patch.Comment != nil {
		e.Comment = patch.Comment
	}

	return s.expenses.Update(ctx, e)
}

func (s *ExpenseService) Delete(ctx context.Context, actor domain.User, id int64) error {
	e, err := s.expenses.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if !actor.IsAdmin() && e.UserID != actor.TelegramID {
		return domain.ErrForbidden
	}
	return s.expenses.Delete(ctx, id)
}

// Last returns the most recently added expense for targetUserID. A member
// can only ever ask about themselves; an admin can ask about anyone, and
// targetUserID == 0 means "myself".
func (s *ExpenseService) Last(ctx context.Context, actor domain.User, targetUserID int64) (*domain.Expense, error) {
	uid := targetUserID
	if !actor.IsAdmin() || uid == 0 {
		uid = actor.TelegramID
	}
	return s.expenses.LastByUser(ctx, uid)
}
