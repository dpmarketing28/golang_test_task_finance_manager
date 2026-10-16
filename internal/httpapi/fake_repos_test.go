package httpapi

import (
	"context"
	"sort"
	"sync"
	"time"

	"test_task_goblin/internal/domain"
)

// fakeUserRepo is an in-memory stand-in for postgres.UserRepo, used so
// httpapi tests exercise the real usecase logic without a real database.
type fakeUserRepo struct {
	mu    sync.Mutex
	users map[int64]domain.User
}

func newFakeUserRepo(users ...domain.User) *fakeUserRepo {
	m := make(map[int64]domain.User, len(users))
	for _, u := range users {
		m[u.TelegramID] = u
	}
	return &fakeUserRepo{users: m}
}

func (r *fakeUserRepo) GetByTelegramID(_ context.Context, id int64) (*domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	u, ok := r.users[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := u
	return &cp, nil
}

func (r *fakeUserRepo) Upsert(_ context.Context, u *domain.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.users[u.TelegramID] = *u
	return nil
}

func (r *fakeUserRepo) List(_ context.Context) ([]domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]domain.User, 0, len(r.users))
	for _, u := range r.users {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TelegramID < out[j].TelegramID })
	return out, nil
}

// fakeExpenseRepo is an in-memory stand-in for postgres.ExpenseRepo.
type fakeExpenseRepo struct {
	mu       sync.Mutex
	nextID   int64
	expenses map[int64]domain.Expense
}

func newFakeExpenseRepo() *fakeExpenseRepo {
	return &fakeExpenseRepo{expenses: make(map[int64]domain.Expense)}
}

func (r *fakeExpenseRepo) Create(_ context.Context, e *domain.Expense) (*domain.Expense, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextID++
	e.ID = r.nextID
	now := time.Now()
	e.CreatedAt = now
	e.UpdatedAt = now
	r.expenses[e.ID] = *e

	cp := *e
	return &cp, nil
}

func (r *fakeExpenseRepo) GetByID(_ context.Context, id int64) (*domain.Expense, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	e, ok := r.expenses[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := e
	return &cp, nil
}

func (r *fakeExpenseRepo) List(_ context.Context, filter domain.ExpenseFilter) ([]domain.Expense, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var out []domain.Expense
	for _, e := range r.expenses {
		if filter.UserID != nil && e.UserID != *filter.UserID {
			continue
		}
		if filter.Category != nil && e.Category != *filter.Category {
			continue
		}
		if filter.From != nil && e.SpentAt.Before(*filter.From) {
			continue
		}
		if filter.To != nil && e.SpentAt.After(*filter.To) {
			continue
		}
		out = append(out, e)
	}

	sort.Slice(out, func(i, j int) bool {
		if !out[i].SpentAt.Equal(out[j].SpentAt) {
			return out[i].SpentAt.After(out[j].SpentAt)
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

func (r *fakeExpenseRepo) Update(_ context.Context, e *domain.Expense) (*domain.Expense, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.expenses[e.ID]; !ok {
		return nil, domain.ErrNotFound
	}
	e.UpdatedAt = time.Now()
	r.expenses[e.ID] = *e

	cp := *e
	return &cp, nil
}

func (r *fakeExpenseRepo) Delete(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.expenses[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.expenses, id)
	return nil
}

func (r *fakeExpenseRepo) LastByUser(_ context.Context, userID int64) (*domain.Expense, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var latest *domain.Expense
	for _, e := range r.expenses {
		if e.UserID != userID {
			continue
		}
		if latest == nil || e.CreatedAt.After(latest.CreatedAt) {
			cp := e
			latest = &cp
		}
	}
	if latest == nil {
		return nil, domain.ErrNotFound
	}
	return latest, nil
}
