package agent

import (
	"context"
	"sort"
	"sync"
	"time"

	"test_task_goblin/internal/domain"
)

// fakeExpenseRepo is an in-memory stand-in for postgres.ExpenseRepo, so
// agent tests exercise the real usecase.ExpenseService logic (including
// its access-control rules) without a real database.
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
	e.CreatedAt, e.UpdatedAt = now, now
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
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
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

// seed inserts an expense directly, bypassing usecase rules, for test
// setup (e.g. "someone else's expense already exists").
func (r *fakeExpenseRepo) seed(e domain.Expense) domain.Expense {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextID++
	e.ID = r.nextID
	now := time.Now()
	e.CreatedAt, e.UpdatedAt = now, now
	r.expenses[e.ID] = e
	return e
}
