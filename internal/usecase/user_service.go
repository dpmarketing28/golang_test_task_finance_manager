package usecase

import (
	"context"
	"errors"
	"fmt"

	"test_task_goblin/internal/domain"
)

// UserService manages family members. There is no sign-up flow: a member
// is created automatically the first time they message the bot.
type UserService struct {
	users           UserRepository
	adminTelegramID int64
}

// NewUserService builds a UserService. adminTelegramID is the Telegram ID
// of the head of the family (from FAMILY_ADMIN_TELEGRAM_ID); 0 means no
// admin has been configured yet, so every new member registers as a
// regular member until it is set.
func NewUserService(users UserRepository, adminTelegramID int64) *UserService {
	return &UserService{users: users, adminTelegramID: adminTelegramID}
}

// EnsureRegistered returns the family member for telegramID, creating
// them on first contact. Role is decided once, at creation time, by
// comparing against adminTelegramID; it is never silently changed
// afterwards, so an existing admin can't accidentally be demoted just
// because the config is later cleared.
func (s *UserService) EnsureRegistered(ctx context.Context, telegramID int64, name string) (*domain.User, error) {
	existing, err := s.users.GetByTelegramID(ctx, telegramID)
	if err == nil {
		if name != "" && existing.Name != name {
			existing.Name = name
			if err := s.users.Upsert(ctx, existing); err != nil {
				return nil, fmt.Errorf("usecase: refresh user %d: %w", telegramID, err)
			}
		}
		return existing, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("usecase: lookup user %d: %w", telegramID, err)
	}

	role := domain.RoleMember
	if s.adminTelegramID != 0 && telegramID == s.adminTelegramID {
		role = domain.RoleAdmin
	}

	u := &domain.User{TelegramID: telegramID, Name: name, Role: role}
	if err := s.users.Upsert(ctx, u); err != nil {
		return nil, fmt.Errorf("usecase: register user %d: %w", telegramID, err)
	}
	return u, nil
}

// List returns all known family members, used by the web panel's member
// filter.
func (s *UserService) List(ctx context.Context) ([]domain.User, error) {
	return s.users.List(ctx)
}
