package domain

import "time"

// Role is a family member's access level.
type Role string

const (
	// RoleMember can only manage their own expenses.
	RoleMember Role = "member"
	// RoleAdmin is the head of the family: can see and edit everyone's
	// expenses, both through the bot and through the web panel.
	RoleAdmin Role = "admin"
)

// User is a family member identified by their Telegram ID.
type User struct {
	TelegramID int64
	Name       string
	Role       Role
	CreatedAt  time.Time
}

// IsAdmin reports whether this user has the head-of-family role.
func (u User) IsAdmin() bool {
	return u.Role == RoleAdmin
}

// SystemActor represents a caller that did not identify as a specific
// family member. It is used for the web panel, which the task explicitly
// says needs no login: an unauthenticated caller is treated as having the
// same full visibility as the family head.
var SystemActor = User{TelegramID: 0, Name: "system", Role: RoleAdmin}
