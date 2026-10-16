package agent

import (
	"context"
	"sort"
	"sync"

	"test_task_goblin/internal/domain"
)

// fakeUserRepo is an in-memory stand-in for postgres.UserRepo, used so
// the list_family_members tool can be tested without a real database.
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
