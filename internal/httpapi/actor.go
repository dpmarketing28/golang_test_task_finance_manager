package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"test_task_goblin/internal/domain"
	"test_task_goblin/internal/usecase"
)

type ctxKey int

const actorCtxKey ctxKey = iota

// actorFromContext returns the caller identified by withActor. It always
// returns a usable value: domain.SystemActor if no actor was attached.
func actorFromContext(ctx context.Context) domain.User {
	if u, ok := ctx.Value(actorCtxKey).(domain.User); ok {
		return u
	}
	return domain.SystemActor
}

// withActor resolves who is calling the API.
//
//   - No X-Telegram-User-Id header: treated as domain.SystemActor (full,
//     admin-level visibility). This is the web panel's case — the task
//     says it needs no login, so an unauthenticated caller is meant to
//     see everything.
//   - Header present: the request is on behalf of that family member
//     (this is how the bot's agent will call these endpoints), scoped by
//     their real role.
func withActor(users usecase.UserRepository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor := domain.SystemActor

			if raw := r.Header.Get("X-Telegram-User-Id"); raw != "" {
				id, err := strconv.ParseInt(raw, 10, 64)
				if err != nil {
					writeError(w, http.StatusBadRequest, "invalid X-Telegram-User-Id header")
					return
				}

				u, err := users.GetByTelegramID(r.Context(), id)
				if errors.Is(err, domain.ErrNotFound) {
					writeError(w, http.StatusForbidden, "unknown telegram user, send /start to the bot first")
					return
				}
				if err != nil {
					writeError(w, http.StatusInternalServerError, "internal error")
					return
				}
				actor = *u
			}

			ctx := context.WithValue(r.Context(), actorCtxKey, actor)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
