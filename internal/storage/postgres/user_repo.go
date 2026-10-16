package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"test_task_goblin/internal/domain"
)

// UserRepo implements usecase.UserRepository on top of Postgres.
type UserRepo struct {
	db *sql.DB
}

func NewUserRepo(db *sql.DB) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) GetByTelegramID(ctx context.Context, id int64) (*domain.User, error) {
	var u domain.User
	err := r.db.QueryRowContext(ctx, `
		SELECT telegram_id, name, role, created_at
		FROM users
		WHERE telegram_id = $1
	`, id).Scan(&u.TelegramID, &u.Name, &u.Role, &u.CreatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: get user %d: %w", id, err)
	}

	return &u, nil
}

// Upsert inserts a new family member or updates an existing one's name
// and role (used both for auto-registration and for refreshing a display
// name coming from Telegram).
func (r *UserRepo) Upsert(ctx context.Context, u *domain.User) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO users (telegram_id, name, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (telegram_id) DO UPDATE SET name = EXCLUDED.name, role = EXCLUDED.role
	`, u.TelegramID, u.Name, u.Role)
	if err != nil {
		return fmt.Errorf("postgres: upsert user %d: %w", u.TelegramID, err)
	}
	return nil
}

func (r *UserRepo) List(ctx context.Context) ([]domain.User, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT telegram_id, name, role, created_at
		FROM users
		ORDER BY created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("postgres: list users: %w", err)
	}
	defer rows.Close()

	var out []domain.User
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.TelegramID, &u.Name, &u.Role, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("postgres: scan user: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
