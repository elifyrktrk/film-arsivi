package domain

import "errors"

// ErrorCode is a stable code; each app shows its own translated message.
type ErrorCode string

const (
	ErrScoreOutOfRange            ErrorCode = "score_out_of_range"
	ErrNoteTooLong                ErrorCode = "note_too_long"
	ErrCategoryNameRequired       ErrorCode = "category_name_required"
	ErrCategoryNameTooLong        ErrorCode = "category_name_too_long"
	ErrCategoryNameTaken          ErrorCode = "category_name_taken"
	ErrCategoryDescriptionTooLong ErrorCode = "category_description_too_long"
	ErrCategoryNotFound           ErrorCode = "category_not_found"
	ErrTitleNotFound              ErrorCode = "title_not_found"
	// ErrInvalidColor has no TS counterpart: zod rejects unknown colors at the type
	// level there, while form input here is untyped.
	ErrInvalidColor ErrorCode = "invalid_color"
)

// Error is a domain rule violation.
type Error struct{ Code ErrorCode }

func (e *Error) Error() string { return string(e.Code) }

func newErr(code ErrorCode) error { return &Error{Code: code} }

// CodeOf returns the domain error code in err, if any.
func CodeOf(err error) (ErrorCode, bool) {
	var de *Error
	if errors.As(err, &de) {
		return de.Code, true
	}
	return "", false
}
