// Package usecase holds business logic: expense CRUD with access control,
// and family-member registration. It knows nothing about Telegram, HTTP or
// Postgres — it only depends on the interfaces below, which the storage
// package implements and the transport packages (telegram, httpapi) call.
package usecase

import (
	"context"

	"test_task_goblin/internal/domain"
)

// UserRepository is the persistence port for family members.
type UserRepository interface {
	GetByTelegramID(ctx context.Context, id int64) (*domain.User, error)
	Upsert(ctx context.Context, u *domain.User) error
	List(ctx context.Context) ([]domain.User, error)
}

// ExpenseRepository is the persistence port for expenses.
type ExpenseRepository interface {
	Create(ctx context.Context, e *domain.Expense) (*domain.Expense, error)
	GetByID(ctx context.Context, id int64) (*domain.Expense, error)
	List(ctx context.Context, filter domain.ExpenseFilter) ([]domain.Expense, error)
	Update(ctx context.Context, e *domain.Expense) (*domain.Expense, error)
	Delete(ctx context.Context, id int64) error
	LastByUser(ctx context.Context, userID int64) (*domain.Expense, error)
}
