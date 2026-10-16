package httpapi

import (
	"net/http"

	"test_task_goblin/internal/domain"
)

// ListCategories handles GET /api/categories. It exists so the web panel
// (and anything else) can populate a category picker from the same
// source of truth as the backend, instead of hardcoding its own copy of
// the list and risking it drifting out of sync.
func ListCategories(w http.ResponseWriter, r *http.Request) {
	out := make([]string, len(domain.AllCategories))
	for i, c := range domain.AllCategories {
		out[i] = string(c)
	}
	writeJSON(w, http.StatusOK, out)
}
