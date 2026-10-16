package agent

import (
	"context"
	"encoding/json"
	"testing"

	"test_task_goblin/internal/domain"
	"test_task_goblin/internal/usecase"
)

var (
	testMember = domain.User{TelegramID: 222, Name: "Сын", Role: domain.RoleMember}
	testAdmin  = domain.User{TelegramID: 111, Name: "Глава семьи", Role: domain.RoleAdmin}
	testOther  = domain.User{TelegramID: 333, Name: "Дочь", Role: domain.RoleMember}
)

func newTestAgent() (*Agent, *fakeExpenseRepo) {
	a, repo, _ := newTestAgentWithUsers(testMember, testAdmin, testOther)
	return a, repo
}

func newTestAgentWithUsers(users ...domain.User) (*Agent, *fakeExpenseRepo, *fakeUserRepo) {
	expenseRepo := newFakeExpenseRepo()
	userRepo := newFakeUserRepo(users...)
	a := &Agent{
		expenses: usecase.NewExpenseService(expenseRepo),
		users:    usecase.NewUserService(userRepo, 0),
		memory:   NewMemory(6),
	}
	return a, expenseRepo, userRepo
}

func decodeResult[T any](t *testing.T, raw string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("decode tool result %q: %v", raw, err)
	}
	return v
}

// --- add_expense ----------------------------------------------------------

func TestToolAddExpense_Success(t *testing.T) {
	a, repo := newTestAgent()

	raw := a.toolAddExpense(context.Background(), testMember, `{"amount":350,"category":"еда","spent_at":"2026-10-16","comment":"кофе"}`)
	out := decodeResult[expenseToolResult](t, raw)

	if !out.OK {
		t.Fatalf("expected ok=true, got %s", raw)
	}
	if out.Expense["category"] != "еда" {
		t.Errorf("category = %v, want еда", out.Expense["category"])
	}
	if len(repo.expenses) != 1 {
		t.Errorf("repo has %d expenses, want 1", len(repo.expenses))
	}
}

// This is the exact scenario from the task: an item that fits none of
// the six specific categories (a drill) must not be rejected outright —
// the model is instructed to fall back to "другое", and the tool layer
// must accept that category like any other.
func TestToolAddExpense_FallbackCategoryOtherIsAccepted(t *testing.T) {
	a, repo := newTestAgent()

	raw := a.toolAddExpense(context.Background(), testMember, `{"amount":4500,"category":"другое","spent_at":"2026-10-16","comment":"дрель"}`)
	out := decodeResult[expenseToolResult](t, raw)

	if !out.OK {
		t.Fatalf("expected ok=true for category 'другое', got %s", raw)
	}
	if len(repo.expenses) != 1 {
		t.Errorf("repo has %d expenses, want 1", len(repo.expenses))
	}
}

func TestToolAddExpense_InvalidCategoryRejected(t *testing.T) {
	a, repo := newTestAgent()

	// A category outside the fixed 7 (the model hallucinating one) must
	// be rejected by the tool layer, not silently accepted — the model
	// should always resolve to "другое" instead, per the system prompt.
	raw := a.toolAddExpense(context.Background(), testMember, `{"amount":100,"category":"инструменты","spent_at":"2026-10-16"}`)
	out := decodeResult[toolErrorResult](t, raw)

	if out.Error == "" {
		t.Errorf("expected an error for an invalid category, got %s", raw)
	}
	if len(repo.expenses) != 0 {
		t.Errorf("repo has %d expenses, want 0 (rejected add must not persist)", len(repo.expenses))
	}
}

func TestToolAddExpense_InvalidDateRejected(t *testing.T) {
	a, _ := newTestAgent()

	raw := a.toolAddExpense(context.Background(), testMember, `{"amount":100,"category":"еда","spent_at":"вчера"}`)
	out := decodeResult[toolErrorResult](t, raw)
	if out.Error == "" {
		t.Errorf("expected an error for a non-YYYY-MM-DD date, got %s", raw)
	}
}

func TestToolAddExpense_InvalidJSONDoesNotPanic(t *testing.T) {
	a, _ := newTestAgent()

	raw := a.toolAddExpense(context.Background(), testMember, `not json`)
	out := decodeResult[toolErrorResult](t, raw)
	if out.Error == "" {
		t.Errorf("expected an error for malformed arguments, got %s", raw)
	}
}

func TestToolAddExpense_MemberCannotOwnSomeoneElse(t *testing.T) {
	a, repo := newTestAgent()

	// Same safety net as the REST API: even if the model is tricked into
	// passing a different owner, a non-admin always ends up owning what
	// they add. add_expense's schema doesn't even expose user_id, but
	// the usecase layer enforces this regardless of what's in the body.
	a.toolAddExpense(context.Background(), testMember, `{"amount":100,"category":"еда","spent_at":"2026-10-16"}`)

	for _, e := range repo.expenses {
		if e.UserID != testMember.TelegramID {
			t.Errorf("expense owner = %d, want %d", e.UserID, testMember.TelegramID)
		}
	}
}

// --- list_expenses ----------------------------------------------------------

func TestToolListExpenses_CategoryFilter(t *testing.T) {
	a, repo := newTestAgent()
	repo.seed(domain.Expense{UserID: testMember.TelegramID, Amount: 350, Category: domain.CategoryFood, SpentAt: mustDate("2026-10-16")})
	repo.seed(domain.Expense{UserID: testMember.TelegramID, Amount: 1200, Category: domain.CategoryTransport, SpentAt: mustDate("2026-10-16")})

	raw := a.toolListExpenses(context.Background(), testMember, `{"category":"транспорт"}`)
	out := decodeResult[listToolResult](t, raw)

	if len(out.Expenses) != 1 || out.Expenses[0]["category"] != "транспорт" {
		t.Errorf("list result = %+v, want exactly one транспорт expense", out.Expenses)
	}
}

func TestToolListExpenses_MemberCannotSeeOthers(t *testing.T) {
	a, repo := newTestAgent()
	repo.seed(domain.Expense{UserID: testOther.TelegramID, Amount: 500, Category: domain.CategoryFood, SpentAt: mustDate("2026-10-16")})

	raw := a.toolListExpenses(context.Background(), testMember, `{}`)
	out := decodeResult[listToolResult](t, raw)

	if len(out.Expenses) != 0 {
		t.Errorf("member saw %d expenses belonging to someone else, want 0", len(out.Expenses))
	}
}

func TestToolListExpenses_EmptyArgsOK(t *testing.T) {
	a, _ := newTestAgent()
	raw := a.toolListExpenses(context.Background(), testMember, "")
	decodeResult[listToolResult](t, raw) // must not error on an empty arguments string
}

// --- get_last_expense -------------------------------------------------------

func TestToolGetLastExpense_NotFoundReturnsNilNotError(t *testing.T) {
	a, _ := newTestAgent()

	raw := a.toolGetLastExpense(context.Background(), testMember, "")
	out := decodeResult[lastExpenseToolResult](t, raw)

	if out.Expense != nil {
		t.Errorf("expected expense=null when none exist, got %s", raw)
	}
}

func TestToolGetLastExpense_ReturnsMostRecent(t *testing.T) {
	a, repo := newTestAgent()
	repo.seed(domain.Expense{UserID: testMember.TelegramID, Amount: 100, Category: domain.CategoryFood, SpentAt: mustDate("2026-10-10")})
	second := repo.seed(domain.Expense{UserID: testMember.TelegramID, Amount: 1200, Category: domain.CategoryTransport, SpentAt: mustDate("2026-10-16")})

	raw := a.toolGetLastExpense(context.Background(), testMember, "")
	out := decodeResult[lastExpenseToolResult](t, raw)

	if int64(out.Expense["id"].(float64)) != second.ID {
		t.Errorf("last expense id = %v, want %d", out.Expense["id"], second.ID)
	}
}

// --- update_expense ----------------------------------------------------------

func TestToolUpdateExpense_Success(t *testing.T) {
	a, repo := newTestAgent()
	e := repo.seed(domain.Expense{UserID: testMember.TelegramID, Amount: 1200, Category: domain.CategoryTransport, SpentAt: mustDate("2026-10-15")})

	raw := a.toolUpdateExpense(context.Background(), testMember, `{"id":`+itoa(e.ID)+`,"amount":1300}`)
	out := decodeResult[expenseToolResult](t, raw)

	if !out.OK || out.Expense["amount"] != 1300.0 {
		t.Errorf("update result = %s, want amount updated to 1300", raw)
	}
}

func TestToolUpdateExpense_MissingIDRejected(t *testing.T) {
	a, _ := newTestAgent()
	raw := a.toolUpdateExpense(context.Background(), testMember, `{"amount":100}`)
	out := decodeResult[toolErrorResult](t, raw)
	if out.Error == "" {
		t.Errorf("expected an error when id is missing, got %s", raw)
	}
}

func TestToolUpdateExpense_ForbiddenForOtherMember(t *testing.T) {
	a, repo := newTestAgent()
	e := repo.seed(domain.Expense{UserID: testMember.TelegramID, Amount: 350, Category: domain.CategoryFood, SpentAt: mustDate("2026-10-16")})

	raw := a.toolUpdateExpense(context.Background(), testOther, `{"id":`+itoa(e.ID)+`,"amount":999}`)
	out := decodeResult[toolErrorResult](t, raw)
	if out.Error == "" {
		t.Errorf("expected a forbidden error, got %s", raw)
	}
}

func TestToolUpdateExpense_AdminCanEditAnyone(t *testing.T) {
	a, repo := newTestAgent()
	e := repo.seed(domain.Expense{UserID: testMember.TelegramID, Amount: 350, Category: domain.CategoryFood, SpentAt: mustDate("2026-10-16")})

	raw := a.toolUpdateExpense(context.Background(), testAdmin, `{"id":`+itoa(e.ID)+`,"amount":450}`)
	out := decodeResult[expenseToolResult](t, raw)
	if !out.OK {
		t.Errorf("expected admin update to succeed, got %s", raw)
	}
}

// --- delete_expense ----------------------------------------------------------

func TestToolDeleteExpense_Success(t *testing.T) {
	a, repo := newTestAgent()
	e := repo.seed(domain.Expense{UserID: testMember.TelegramID, Amount: 1200, Category: domain.CategoryTransport, SpentAt: mustDate("2026-10-15")})

	raw := a.toolDeleteExpense(context.Background(), testMember, `{"id":`+itoa(e.ID)+`}`)
	out := decodeResult[expenseToolResult](t, raw)
	if !out.OK {
		t.Errorf("expected ok=true, got %s", raw)
	}
	if _, exists := repo.expenses[e.ID]; exists {
		t.Errorf("expense %d still exists after delete", e.ID)
	}
}

func TestToolDeleteExpense_NotFound(t *testing.T) {
	a, _ := newTestAgent()
	raw := a.toolDeleteExpense(context.Background(), testMember, `{"id":999999}`)
	out := decodeResult[toolErrorResult](t, raw)
	if out.Error == "" {
		t.Errorf("expected an error for a missing expense, got %s", raw)
	}
}

func TestToolDeleteExpense_ForbiddenForOtherMember(t *testing.T) {
	a, repo := newTestAgent()
	e := repo.seed(domain.Expense{UserID: testMember.TelegramID, Amount: 350, Category: domain.CategoryFood, SpentAt: mustDate("2026-10-16")})

	raw := a.toolDeleteExpense(context.Background(), testOther, `{"id":`+itoa(e.ID)+`}`)
	out := decodeResult[toolErrorResult](t, raw)
	if out.Error == "" {
		t.Errorf("expected a forbidden error, got %s", raw)
	}
	if _, exists := repo.expenses[e.ID]; !exists {
		t.Errorf("expense %d was deleted despite missing permission", e.ID)
	}
}

// --- cross-member access for admins (user_id scoping) ------------------------

// This is the fix for the code-review finding: without user_id, an
// admin's list_expenses would silently return the WHOLE FAMILY's
// expenses combined, with no way to tell them apart or to ask about just
// themselves or one specific person.
func TestToolListExpenses_AdminCanScopeToSelf(t *testing.T) {
	a, repo := newTestAgent()
	repo.seed(domain.Expense{UserID: testAdmin.TelegramID, Amount: 100, Category: domain.CategoryFood, SpentAt: mustDate("2026-10-16")})
	repo.seed(domain.Expense{UserID: testMember.TelegramID, Amount: 500, Category: domain.CategoryFood, SpentAt: mustDate("2026-10-16")})

	raw := a.toolListExpenses(context.Background(), testAdmin, `{"user_id":`+itoa(testAdmin.TelegramID)+`}`)
	out := decodeResult[listToolResult](t, raw)

	if len(out.Expenses) != 1 || int64(out.Expenses[0]["user_id"].(float64)) != testAdmin.TelegramID {
		t.Errorf("admin scoped to self got %+v, want exactly their own expense", out.Expenses)
	}
}

func TestToolListExpenses_AdminOmittingUserIDSeesEveryoneWithOwnerTagged(t *testing.T) {
	a, repo := newTestAgent()
	repo.seed(domain.Expense{UserID: testAdmin.TelegramID, Amount: 100, Category: domain.CategoryFood, SpentAt: mustDate("2026-10-16")})
	repo.seed(domain.Expense{UserID: testMember.TelegramID, Amount: 500, Category: domain.CategoryFood, SpentAt: mustDate("2026-10-16")})

	raw := a.toolListExpenses(context.Background(), testAdmin, `{}`)
	out := decodeResult[listToolResult](t, raw)

	if len(out.Expenses) != 2 {
		t.Fatalf("admin without user_id should see everyone, got %+v", out.Expenses)
	}
	seenOwners := map[int64]bool{}
	for _, e := range out.Expenses {
		seenOwners[int64(e["user_id"].(float64))] = true
	}
	if !seenOwners[testAdmin.TelegramID] || !seenOwners[testMember.TelegramID] {
		t.Errorf("each expense must carry its real owner's user_id so results aren't misattributed, got %+v", out.Expenses)
	}
}

func TestToolGetLastExpense_AdminCanTargetAnotherMember(t *testing.T) {
	a, repo := newTestAgent()
	repo.seed(domain.Expense{UserID: testAdmin.TelegramID, Amount: 100, Category: domain.CategoryFood, SpentAt: mustDate("2026-10-16")})
	target := repo.seed(domain.Expense{UserID: testMember.TelegramID, Amount: 500, Category: domain.CategoryFood, SpentAt: mustDate("2026-10-16")})

	raw := a.toolGetLastExpense(context.Background(), testAdmin, `{"user_id":`+itoa(testMember.TelegramID)+`}`)
	out := decodeResult[lastExpenseToolResult](t, raw)

	if int64(out.Expense["id"].(float64)) != target.ID {
		t.Errorf("last expense id = %v, want %d (the targeted member's)", out.Expense["id"], target.ID)
	}
}

func TestToolGetLastExpense_MemberCannotTargetAnotherMember(t *testing.T) {
	a, repo := newTestAgent()
	repo.seed(domain.Expense{UserID: testOther.TelegramID, Amount: 500, Category: domain.CategoryFood, SpentAt: mustDate("2026-10-16")})

	// testMember has no expenses of their own; asking for testOther's
	// last expense must not leak it — usecase.Last ignores the
	// requested target for a non-admin actor.
	raw := a.toolGetLastExpense(context.Background(), testMember, `{"user_id":`+itoa(testOther.TelegramID)+`}`)
	out := decodeResult[lastExpenseToolResult](t, raw)

	if out.Expense != nil {
		t.Errorf("member leaked another member's last expense: %s", raw)
	}
}

func TestToolAddExpense_AdminCanAddForAnotherMember(t *testing.T) {
	a, _ := newTestAgent()

	raw := a.toolAddExpense(context.Background(), testAdmin, `{"user_id":`+itoa(testMember.TelegramID)+`,"amount":300,"category":"еда","spent_at":"2026-10-16"}`)
	out := decodeResult[expenseToolResult](t, raw)

	if !out.OK {
		t.Fatalf("expected ok=true, got %s", raw)
	}
	if int64(out.Expense["user_id"].(float64)) != testMember.TelegramID {
		t.Errorf("expense owner = %v, want %d (admin logging on someone else's behalf)", out.Expense["user_id"], testMember.TelegramID)
	}
}

// --- list_family_members -----------------------------------------------------

func TestToolListFamilyMembers_ReturnsEveryone(t *testing.T) {
	a, _, _ := newTestAgentWithUsers(testAdmin, testMember, testOther)

	raw := a.toolListFamilyMembers(context.Background())
	out := decodeResult[membersToolResult](t, raw)

	if len(out.Members) != 3 {
		t.Fatalf("members = %+v, want 3", out.Members)
	}

	names := map[string]bool{}
	for _, m := range out.Members {
		names[m["name"].(string)] = true
	}
	if !names[testMember.Name] || !names[testAdmin.Name] || !names[testOther.Name] {
		t.Errorf("expected all three family members by name, got %+v", out.Members)
	}
}

// --- tool schema consistency -------------------------------------------------

// Guards against the tool schema silently drifting from the domain's
// actual list of categories (e.g. someone adds a category in domain but
// forgets the schema, or vice versa).
func TestCategoryEnum_MatchesDomainCategories(t *testing.T) {
	got := categoryEnum()
	if len(got) != len(domain.AllCategories) {
		t.Fatalf("categoryEnum() has %d entries, domain.AllCategories has %d", len(got), len(domain.AllCategories))
	}
	for i, c := range domain.AllCategories {
		if got[i] != string(c) {
			t.Errorf("categoryEnum()[%d] = %q, want %q", i, got[i], string(c))
		}
	}
}
