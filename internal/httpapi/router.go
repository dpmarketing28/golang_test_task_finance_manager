// Package httpapi is the REST API: CRUD over expenses plus a read-only
// member list. It is the single boundary used both by the web panel and
// by the Telegram agent's tool calls, so expense rules live in one place
// (internal/usecase) instead of being duplicated per transport.
package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"test_task_goblin/internal/usecase"
)

// NewRouter wires the REST API. users is passed separately from userSvc
// because the actor-resolving middleware only needs read access, not the
// registration logic.
func NewRouter(expenseSvc *usecase.ExpenseService, userSvc *usecase.UserService, users usecase.UserRepository) http.Handler {
	expenseHandler := NewExpenseHandler(expenseSvc)
	userHandler := NewUserHandler(userSvc)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(withActor(users))

	r.Route("/api", func(r chi.Router) {
		r.Get("/users", userHandler.List)

		r.Route("/expenses", func(r chi.Router) {
			r.Post("/", expenseHandler.Create)
			r.Get("/", expenseHandler.List)
			r.Get("/last", expenseHandler.Last)

			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", expenseHandler.Get)
				r.Patch("/", expenseHandler.Update)
				r.Delete("/", expenseHandler.Delete)
			})
		})
	})

	return r
}
