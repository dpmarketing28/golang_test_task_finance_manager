package httpapi

import (
	"net/http"

	"test_task_goblin/internal/usecase"
)

// UserHandler exposes read access to family members, used by the web
// panel's member filter and by the agent to resolve names to Telegram IDs.
type UserHandler struct {
	users *usecase.UserService
}

func NewUserHandler(s *usecase.UserService) *UserHandler {
	return &UserHandler{users: s}
}

type userResponse struct {
	TelegramID int64  `json:"telegram_id"`
	Name       string `json:"name"`
	Role       string `json:"role"`
}

// List handles GET /api/users.
func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	users, err := h.users.List(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}

	out := make([]userResponse, 0, len(users))
	for _, u := range users {
		out = append(out, userResponse{TelegramID: u.TelegramID, Name: u.Name, Role: string(u.Role)})
	}
	writeJSON(w, http.StatusOK, out)
}
