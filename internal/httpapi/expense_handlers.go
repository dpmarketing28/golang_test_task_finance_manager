package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"test_task_goblin/internal/domain"
	"test_task_goblin/internal/usecase"
)

const dateLayout = "2006-01-02"

// ExpenseHandler exposes expense CRUD over HTTP. These are the endpoints
// both the web panel and, later, the LLM agent's tool calls will hit.
type ExpenseHandler struct {
	expenses *usecase.ExpenseService
}

func NewExpenseHandler(s *usecase.ExpenseService) *ExpenseHandler {
	return &ExpenseHandler{expenses: s}
}

type expenseRequest struct {
	UserID   int64   `json:"user_id,omitempty"`
	Amount   float64 `json:"amount"`
	Category string  `json:"category"`
	SpentAt  string  `json:"spent_at"` // YYYY-MM-DD
	Comment  *string `json:"comment,omitempty"`
}

type expenseResponse struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Amount    float64   `json:"amount"`
	Category  string    `json:"category"`
	SpentAt   string    `json:"spent_at"`
	Comment   *string   `json:"comment,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toExpenseResponse(e domain.Expense) expenseResponse {
	return expenseResponse{
		ID:        e.ID,
		UserID:    e.UserID,
		Amount:    e.Amount,
		Category:  string(e.Category),
		SpentAt:   e.SpentAt.Format(dateLayout),
		Comment:   e.Comment,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
}

// Create handles POST /api/expenses.
func (h *ExpenseHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req expenseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	spentAt, err := time.Parse(dateLayout, req.SpentAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid spent_at, expected YYYY-MM-DD")
		return
	}

	actor := actorFromContext(r.Context())
	e, err := h.expenses.Add(r.Context(), actor, usecase.AddExpenseInput{
		UserID:   req.UserID,
		Amount:   req.Amount,
		Category: domain.Category(req.Category),
		SpentAt:  spentAt,
		Comment:  req.Comment,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toExpenseResponse(*e))
}

// List handles GET /api/expenses?user_id=&category=&from=&to=.
func (h *ExpenseHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := domain.ExpenseFilter{}

	if v := q.Get("user_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user_id")
			return
		}
		filter.UserID = &id
	}
	if v := q.Get("category"); v != "" {
		c := domain.Category(v)
		filter.Category = &c
	}
	if v := q.Get("from"); v != "" {
		d, err := time.Parse(dateLayout, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid from, expected YYYY-MM-DD")
			return
		}
		filter.From = &d
	}
	if v := q.Get("to"); v != "" {
		d, err := time.Parse(dateLayout, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid to, expected YYYY-MM-DD")
			return
		}
		filter.To = &d
	}

	actor := actorFromContext(r.Context())
	expenses, err := h.expenses.List(r.Context(), actor, filter)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	out := make([]expenseResponse, 0, len(expenses))
	for _, e := range expenses {
		out = append(out, toExpenseResponse(e))
	}
	writeJSON(w, http.StatusOK, out)
}

// Get handles GET /api/expenses/{id}.
func (h *ExpenseHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := idFromPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	actor := actorFromContext(r.Context())
	e, err := h.expenses.Get(r.Context(), actor, id)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toExpenseResponse(*e))
}

type expensePatchRequest struct {
	Amount   *float64 `json:"amount,omitempty"`
	Category *string  `json:"category,omitempty"`
	SpentAt  *string  `json:"spent_at,omitempty"`
	Comment  *string  `json:"comment,omitempty"`
}

// Update handles PATCH /api/expenses/{id}.
func (h *ExpenseHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := idFromPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var req expensePatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	patch := usecase.UpdateExpenseInput{Amount: req.Amount, Comment: req.Comment}
	if req.Category != nil {
		c := domain.Category(*req.Category)
		patch.Category = &c
	}
	if req.SpentAt != nil {
		d, err := time.Parse(dateLayout, *req.SpentAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid spent_at, expected YYYY-MM-DD")
			return
		}
		patch.SpentAt = &d
	}

	actor := actorFromContext(r.Context())
	e, err := h.expenses.Update(r.Context(), actor, id, patch)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toExpenseResponse(*e))
}

// Delete handles DELETE /api/expenses/{id}.
func (h *ExpenseHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := idFromPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	actor := actorFromContext(r.Context())
	if err := h.expenses.Delete(r.Context(), actor, id); err != nil {
		handleServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Last handles GET /api/expenses/last?user_id=. It backs the bot's
// "last one was not 350, but 450" natural-language edit: the agent first
// calls this to resolve which expense "last one" refers to.
func (h *ExpenseHandler) Last(w http.ResponseWriter, r *http.Request) {
	actor := actorFromContext(r.Context())

	var targetID int64
	if v := r.URL.Query().Get("user_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user_id")
			return
		}
		targetID = id
	}

	e, err := h.expenses.Last(r.Context(), actor, targetID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toExpenseResponse(*e))
}

func idFromPath(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}
