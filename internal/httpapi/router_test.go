package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"test_task_goblin/internal/domain"
	"test_task_goblin/internal/usecase"
)

var (
	admin  = domain.User{TelegramID: 111, Name: "Глава семьи", Role: domain.RoleAdmin}
	member = domain.User{TelegramID: 222, Name: "Сын", Role: domain.RoleMember}
	other  = domain.User{TelegramID: 333, Name: "Дочь", Role: domain.RoleMember}
)

// newTestRouter builds a router backed by fresh in-memory fakes seeded
// with the given family members.
func newTestRouter(users ...domain.User) http.Handler {
	userRepo := newFakeUserRepo(users...)
	expenseRepo := newFakeExpenseRepo()
	userSvc := usecase.NewUserService(userRepo, 0)
	expenseSvc := usecase.NewExpenseService(expenseRepo)
	return NewRouter(expenseSvc, userSvc, userRepo)
}

// doRequest issues a request with no X-Telegram-User-Id header (the web
// panel's case: unauthenticated, full admin-level visibility).
func doRequest(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return doRequestAs(t, h, method, path, 0, false, body)
}

// doRequestAs issues a request on behalf of a specific Telegram user.
func doRequestAs(t *testing.T, h http.Handler, method, path string, actorID int64, withHeader bool, body any) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(b)
	}

	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if withHeader {
		req.Header.Set("X-Telegram-User-Id", strconv.FormatInt(actorID, 10))
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
	return v
}

func createExpense(t *testing.T, h http.Handler, actorID int64, req expenseRequest) expenseResponse {
	t.Helper()
	rec := doRequestAs(t, h, http.MethodPost, "/api/expenses", actorID, true, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create expense: want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	return decode[expenseResponse](t, rec)
}

// --- Create -----------------------------------------------------------

func TestCreateExpense_MemberOwnsItRegardlessOfRequestedUserID(t *testing.T) {
	h := newTestRouter(admin, member)

	// member tries to log the expense under someone else's id — must be
	// ignored, member always owns what they create.
	got := createExpense(t, h, member.TelegramID, expenseRequest{
		UserID: admin.TelegramID, Amount: 350, Category: "еда", SpentAt: "2026-10-16", Comment: strPtr("кофе"),
	})

	if got.UserID != member.TelegramID {
		t.Errorf("expense owner = %d, want %d (member can't assign ownership to someone else)", got.UserID, member.TelegramID)
	}
}

func TestCreateExpense_AdminCanLogForAnotherMember(t *testing.T) {
	h := newTestRouter(admin, member)

	got := createExpense(t, h, admin.TelegramID, expenseRequest{
		UserID: member.TelegramID, Amount: 100, Category: "покупки", SpentAt: "2026-10-16",
	})

	if got.UserID != member.TelegramID {
		t.Errorf("expense owner = %d, want %d (admin should be able to log for someone else)", got.UserID, member.TelegramID)
	}
}

func TestCreateExpense_InvalidCategoryRejected(t *testing.T) {
	h := newTestRouter(member)

	rec := doRequestAs(t, h, http.MethodPost, "/api/expenses", member.TelegramID, true, expenseRequest{
		Amount: 100, Category: "бензин", SpentAt: "2026-10-16",
	})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateExpense_NonPositiveAmountRejected(t *testing.T) {
	h := newTestRouter(member)

	rec := doRequestAs(t, h, http.MethodPost, "/api/expenses", member.TelegramID, true, expenseRequest{
		Amount: 0, Category: "еда", SpentAt: "2026-10-16",
	})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateExpense_InvalidDateRejected(t *testing.T) {
	h := newTestRouter(member)

	rec := doRequestAs(t, h, http.MethodPost, "/api/expenses", member.TelegramID, true, expenseRequest{
		Amount: 100, Category: "еда", SpentAt: "16-10-2026",
	})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateExpense_UnknownActorForbidden(t *testing.T) {
	h := newTestRouter(admin) // 999 never registered (never sent /start)

	rec := doRequestAs(t, h, http.MethodPost, "/api/expenses", 999, true, expenseRequest{
		Amount: 100, Category: "еда", SpentAt: "2026-10-16",
	})

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403: %s", rec.Code, rec.Body.String())
	}
}

// --- List ---------------------------------------------------------------

func TestListExpenses_MemberSeesOnlyOwn(t *testing.T) {
	h := newTestRouter(admin, member, other)

	createExpense(t, h, member.TelegramID, expenseRequest{Amount: 350, Category: "еда", SpentAt: "2026-10-16"})
	createExpense(t, h, other.TelegramID, expenseRequest{Amount: 500, Category: "еда", SpentAt: "2026-10-16"})

	rec := doRequestAs(t, h, http.MethodGet, "/api/expenses", member.TelegramID, true, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	list := decode[[]expenseResponse](t, rec)
	if len(list) != 1 || list[0].UserID != member.TelegramID {
		t.Errorf("member's list = %+v, want exactly one expense owned by %d", list, member.TelegramID)
	}
}

func TestListExpenses_MemberCannotEscapeScopeViaUserIDParam(t *testing.T) {
	h := newTestRouter(admin, member, other)

	createExpense(t, h, other.TelegramID, expenseRequest{Amount: 500, Category: "еда", SpentAt: "2026-10-16"})

	// member explicitly asks for other's expenses — should still only see their own (none).
	rec := doRequestAs(t, h, http.MethodGet, "/api/expenses?user_id="+strconv.FormatInt(other.TelegramID, 10), member.TelegramID, true, nil)
	list := decode[[]expenseResponse](t, rec)

	if len(list) != 0 {
		t.Errorf("member bypassed scoping via user_id param, got %+v", list)
	}
}

func TestListExpenses_AdminSeesEverything(t *testing.T) {
	h := newTestRouter(admin, member, other)

	createExpense(t, h, member.TelegramID, expenseRequest{Amount: 350, Category: "еда", SpentAt: "2026-10-16"})
	createExpense(t, h, other.TelegramID, expenseRequest{Amount: 500, Category: "транспорт", SpentAt: "2026-10-16"})

	rec := doRequestAs(t, h, http.MethodGet, "/api/expenses", admin.TelegramID, true, nil)
	list := decode[[]expenseResponse](t, rec)

	if len(list) != 2 {
		t.Errorf("admin's list has %d items, want 2: %+v", len(list), list)
	}
}

func TestListExpenses_NoHeaderActsAsAdmin(t *testing.T) {
	h := newTestRouter(admin, member)
	createExpense(t, h, member.TelegramID, expenseRequest{Amount: 350, Category: "еда", SpentAt: "2026-10-16"})

	rec := doRequest(t, h, http.MethodGet, "/api/expenses", nil) // web panel: no auth header
	list := decode[[]expenseResponse](t, rec)

	if len(list) != 1 {
		t.Errorf("unauthenticated (web panel) call should see everything, got %+v", list)
	}
}

func TestListExpenses_CategoryFilter(t *testing.T) {
	h := newTestRouter(admin)
	createExpense(t, h, admin.TelegramID, expenseRequest{Amount: 350, Category: "еда", SpentAt: "2026-10-16"})
	createExpense(t, h, admin.TelegramID, expenseRequest{Amount: 1200, Category: "транспорт", SpentAt: "2026-10-16"})

	rec := doRequestAs(t, h, http.MethodGet, "/api/expenses?category=транспорт", admin.TelegramID, true, nil)
	list := decode[[]expenseResponse](t, rec)

	if len(list) != 1 || list[0].Category != "транспорт" {
		t.Errorf("category filter result = %+v, want exactly one 'транспорт' expense", list)
	}
}

// --- Get / Update / Delete ----------------------------------------------

func TestGetExpense_ForbiddenForOtherMember(t *testing.T) {
	h := newTestRouter(member, other)
	e := createExpense(t, h, member.TelegramID, expenseRequest{Amount: 350, Category: "еда", SpentAt: "2026-10-16"})

	rec := doRequestAs(t, h, http.MethodGet, "/api/expenses/"+strconv.FormatInt(e.ID, 10), other.TelegramID, true, nil)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403: %s", rec.Code, rec.Body.String())
	}
}

func TestGetExpense_NotFound(t *testing.T) {
	h := newTestRouter(admin)

	rec := doRequestAs(t, h, http.MethodGet, "/api/expenses/999999", admin.TelegramID, true, nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateExpense_OwnerCanPatchAmount(t *testing.T) {
	h := newTestRouter(member)
	e := createExpense(t, h, member.TelegramID, expenseRequest{Amount: 350, Category: "еда", SpentAt: "2026-10-16"})

	newAmount := 450.0
	rec := doRequestAs(t, h, http.MethodPatch, "/api/expenses/"+strconv.FormatInt(e.ID, 10), member.TelegramID, true,
		expensePatchRequest{Amount: &newAmount})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	got := decode[expenseResponse](t, rec)
	if got.Amount != 450 {
		t.Errorf("amount = %v, want 450", got.Amount)
	}
}

func TestUpdateExpense_ForbiddenForOtherMember(t *testing.T) {
	h := newTestRouter(member, other)
	e := createExpense(t, h, member.TelegramID, expenseRequest{Amount: 350, Category: "еда", SpentAt: "2026-10-16"})

	newAmount := 999.0
	rec := doRequestAs(t, h, http.MethodPatch, "/api/expenses/"+strconv.FormatInt(e.ID, 10), other.TelegramID, true,
		expensePatchRequest{Amount: &newAmount})

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateExpense_AdminCanPatchAnyonesExpense(t *testing.T) {
	h := newTestRouter(admin, member)
	e := createExpense(t, h, member.TelegramID, expenseRequest{Amount: 350, Category: "еда", SpentAt: "2026-10-16"})

	newAmount := 450.0
	rec := doRequestAs(t, h, http.MethodPatch, "/api/expenses/"+strconv.FormatInt(e.ID, 10), admin.TelegramID, true,
		expensePatchRequest{Amount: &newAmount})

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteExpense_ForbiddenThenAdminSucceeds(t *testing.T) {
	h := newTestRouter(admin, member, other)
	e := createExpense(t, h, member.TelegramID, expenseRequest{Amount: 350, Category: "еда", SpentAt: "2026-10-16"})

	id := strconv.FormatInt(e.ID, 10)

	rec := doRequestAs(t, h, http.MethodDelete, "/api/expenses/"+id, other.TelegramID, true, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("other member delete: status = %d, want 403: %s", rec.Code, rec.Body.String())
	}

	rec = doRequestAs(t, h, http.MethodDelete, "/api/expenses/"+id, admin.TelegramID, true, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("admin delete: status = %d, want 204: %s", rec.Code, rec.Body.String())
	}

	rec = doRequestAs(t, h, http.MethodGet, "/api/expenses/"+id, admin.TelegramID, true, nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expense still exists after delete: status = %d", rec.Code)
	}
}

func TestExpenseEndpoints_InvalidIDRejected(t *testing.T) {
	h := newTestRouter(admin)

	for _, method := range []string{http.MethodGet, http.MethodPatch, http.MethodDelete} {
		rec := doRequestAs(t, h, method, "/api/expenses/not-a-number", admin.TelegramID, true, nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s /api/expenses/not-a-number: status = %d, want 400", method, rec.Code)
		}
	}
}

// --- Last ----------------------------------------------------------------

func TestLastExpense_ReturnsMostRecentlyCreated(t *testing.T) {
	h := newTestRouter(member)
	createExpense(t, h, member.TelegramID, expenseRequest{Amount: 100, Category: "еда", SpentAt: "2026-10-10"})
	second := createExpense(t, h, member.TelegramID, expenseRequest{Amount: 350, Category: "еда", SpentAt: "2026-10-16"})

	rec := doRequestAs(t, h, http.MethodGet, "/api/expenses/last", member.TelegramID, true, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	got := decode[expenseResponse](t, rec)
	if got.ID != second.ID {
		t.Errorf("last expense id = %d, want %d (the most recently created one)", got.ID, second.ID)
	}
}

func TestLastExpense_MemberCannotQueryAnotherUsersLast(t *testing.T) {
	h := newTestRouter(member, other)
	createExpense(t, h, other.TelegramID, expenseRequest{Amount: 500, Category: "еда", SpentAt: "2026-10-16"})

	rec := doRequestAs(t, h, http.MethodGet, "/api/expenses/last?user_id="+strconv.FormatInt(other.TelegramID, 10), member.TelegramID, true, nil)

	// member has no expenses of their own, and the user_id param must be
	// ignored for a non-admin, so this should resolve to "member has no
	// expenses yet" (404), not other's expense.
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (scope must stay on member, not leak other's last expense): %s", rec.Code, rec.Body.String())
	}
}

// --- Users ----------------------------------------------------------------

func TestListUsers(t *testing.T) {
	h := newTestRouter(admin, member)

	rec := doRequest(t, h, http.MethodGet, "/api/users", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	list := decode[[]userResponse](t, rec)
	if len(list) != 2 {
		t.Errorf("users list = %+v, want 2", list)
	}
}

func strPtr(s string) *string { return &s }
