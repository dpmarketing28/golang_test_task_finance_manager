package domain

import "time"

// Category is a fixed expense category, exactly as listed in the task.
type Category string

const (
	CategoryFood          Category = "еда"
	CategoryTransport     Category = "транспорт"
	CategoryHousing       Category = "жильё"
	CategoryEntertainment Category = "развлечения"
	CategoryHealth        Category = "здоровье"
	CategoryShopping      Category = "покупки"
	CategoryOther         Category = "другое"
)

// categories is the closed set of valid categories.
var categories = map[Category]bool{
	CategoryFood:          true,
	CategoryTransport:     true,
	CategoryHousing:       true,
	CategoryEntertainment: true,
	CategoryHealth:        true,
	CategoryShopping:      true,
	CategoryOther:         true,
}

// Valid reports whether c is one of the known categories.
func (c Category) Valid() bool {
	return categories[c]
}

// Expense is a single spending record belonging to one family member.
type Expense struct {
	ID        int64
	UserID    int64 // owner's Telegram ID
	Amount    float64
	Category  Category
	SpentAt   time.Time // date only, no time-of-day
	Comment   *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ExpenseFilter narrows an expense listing. A nil field means "no filter
// on this dimension".
type ExpenseFilter struct {
	UserID   *int64
	Category *Category
	From     *time.Time
	To       *time.Time
}
