package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"test_task_goblin/internal/domain"
)

// ExpenseRepo implements usecase.ExpenseRepository on top of Postgres.
type ExpenseRepo struct {
	db *sql.DB
}

func NewExpenseRepo(db *sql.DB) *ExpenseRepo {
	return &ExpenseRepo{db: db}
}

func (r *ExpenseRepo) Create(ctx context.Context, e *domain.Expense) (*domain.Expense, error) {
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO expenses (user_id, amount, category, spent_at, comment)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
	`, e.UserID, e.Amount, e.Category, e.SpentAt, e.Comment).Scan(&e.ID, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("postgres: create expense: %w", err)
	}
	return e, nil
}

func (r *ExpenseRepo) GetByID(ctx context.Context, id int64) (*domain.Expense, error) {
	e := &domain.Expense{}
	err := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, amount, category, spent_at, comment, created_at, updated_at
		FROM expenses
		WHERE id = $1
	`, id).Scan(&e.ID, &e.UserID, &e.Amount, &e.Category, &e.SpentAt, &e.Comment, &e.CreatedAt, &e.UpdatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: get expense %d: %w", id, err)
	}

	return e, nil
}

// List returns expenses matching filter, newest first. Filter fields left
// nil are not applied, so an admin with an empty filter gets everyone's
// expenses.
func (r *ExpenseRepo) List(ctx context.Context, filter domain.ExpenseFilter) ([]domain.Expense, error) {
	query := `
		SELECT id, user_id, amount, category, spent_at, comment, created_at, updated_at
		FROM expenses
		WHERE 1 = 1
	`
	var args []any

	if filter.UserID != nil {
		args = append(args, *filter.UserID)
		query += fmt.Sprintf(" AND user_id = $%d", len(args))
	}
	if filter.Category != nil {
		args = append(args, *filter.Category)
		query += fmt.Sprintf(" AND category = $%d", len(args))
	}
	if filter.From != nil {
		args = append(args, *filter.From)
		query += fmt.Sprintf(" AND spent_at >= $%d", len(args))
	}
	if filter.To != nil {
		args = append(args, *filter.To)
		query += fmt.Sprintf(" AND spent_at <= $%d", len(args))
	}
	query += " ORDER BY spent_at DESC, id DESC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list expenses: %w", err)
	}
	defer rows.Close()

	var out []domain.Expense
	for rows.Next() {
		var e domain.Expense
		if err := rows.Scan(&e.ID, &e.UserID, &e.Amount, &e.Category, &e.SpentAt, &e.Comment, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("postgres: scan expense: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *ExpenseRepo) Update(ctx context.Context, e *domain.Expense) (*domain.Expense, error) {
	err := r.db.QueryRowContext(ctx, `
		UPDATE expenses
		SET amount = $1, category = $2, spent_at = $3, comment = $4, updated_at = now()
		WHERE id = $5
		RETURNING updated_at
	`, e.Amount, e.Category, e.SpentAt, e.Comment, e.ID).Scan(&e.UpdatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: update expense %d: %w", e.ID, err)
	}

	return e, nil
}

func (r *ExpenseRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM expenses WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("postgres: delete expense %d: %w", id, err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("postgres: delete expense %d: rows affected: %w", id, err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}

	return nil
}

// LastByUser returns the most recently created expense for a user. It
// backs the bot's "last one was not 350, but 450" edit, which refers to
// the most recent entry, not the one with the latest spent_at date.
func (r *ExpenseRepo) LastByUser(ctx context.Context, userID int64) (*domain.Expense, error) {
	e := &domain.Expense{}
	err := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, amount, category, spent_at, comment, created_at, updated_at
		FROM expenses
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, userID).Scan(&e.ID, &e.UserID, &e.Amount, &e.Category, &e.SpentAt, &e.Comment, &e.CreatedAt, &e.UpdatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: last expense for user %d: %w", userID, err)
	}

	return e, nil
}
