package domain

import "fmt"

type ErrorCode string

const (
	CodeValidation ErrorCode = "validation_error"
	CodeNotFound   ErrorCode = "not_found"
	CodeConflict   ErrorCode = "conflict"
	CodeState      ErrorCode = "invalid_state"
	CodeIntegrity  ErrorCode = "integrity_error"
)

type Error struct {
	Code    ErrorCode `json:"code"`
	Field   string    `json:"field,omitempty"`
	Message string    `json:"message"`
}

func (e *Error) Error() string { return e.Message }

func Validation(field, message string) error {
	return &Error{Code: CodeValidation, Field: field, Message: message}
}

func Conflict(message string) error     { return &Error{Code: CodeConflict, Message: message} }
func InvalidState(message string) error { return &Error{Code: CodeState, Message: message} }
func NotFound(kind, id string) error {
	return &Error{Code: CodeNotFound, Message: fmt.Sprintf("%s %s 不存在", kind, id)}
}
func Integrity(message string) error { return &Error{Code: CodeIntegrity, Message: message} }
