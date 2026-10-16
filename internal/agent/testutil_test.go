package agent

import (
	"strconv"
	"time"
)

func mustDate(s string) time.Time {
	d, err := time.Parse(dateLayout, s)
	if err != nil {
		panic(err) // test setup only, never a real input
	}
	return d
}

func itoa(id int64) string {
	return strconv.FormatInt(id, 10)
}

// Named result shapes shared across tests, instead of repeating the same
// anonymous struct literal in every test function.
type toolErrorResult struct {
	Error string `json:"error"`
}

type expenseToolResult struct {
	OK      bool           `json:"ok"`
	Expense map[string]any `json:"expense"`
}

type listToolResult struct {
	Expenses []map[string]any `json:"expenses"`
}

type lastExpenseToolResult struct {
	Expense map[string]any `json:"expense"`
}

type membersToolResult struct {
	Members []map[string]any `json:"members"`
}
