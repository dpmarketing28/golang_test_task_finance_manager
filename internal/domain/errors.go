package domain

import "errors"

// Sentinel errors returned by repositories and usecases. Callers use
// errors.Is to branch on them (e.g. to map to HTTP status codes).
var (
	// ErrNotFound means the requested entity does not exist.
	ErrNotFound = errors.New("not found")
	// ErrForbidden means the acting user is not allowed to touch this
	// entity (e.g. a member trying to edit someone else's expense).
	ErrForbidden = errors.New("forbidden")
	// ErrInvalidInput means the request failed validation (bad category,
	// non-positive amount, etc).
	ErrInvalidInput = errors.New("invalid input")
)
