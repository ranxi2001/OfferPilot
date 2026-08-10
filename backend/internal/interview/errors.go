package interview

import (
	"errors"
	"fmt"
)

type ErrorCode string

const (
	CodeValidation   ErrorCode = "validation"
	CodeNotFound     ErrorCode = "not_found"
	CodeConflict     ErrorCode = "conflict"
	CodeInvalidState ErrorCode = "invalid_state"
	CodeGrounding    ErrorCode = "grounding"
	CodeUnavailable  ErrorCode = "service_unavailable"
	CodeInternal     ErrorCode = "internal"
)

var (
	ErrStoreNotFound = errors.New("interview store: not found")
	ErrStoreConflict = errors.New("interview store: version conflict")
)

type DomainError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Field   string    `json:"field,omitempty"`
	Cause   error     `json:"-"`
}

func (e *DomainError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s (%s)", e.Code, e.Message, e.Field)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *DomainError) Unwrap() error { return e.Cause }

func IsCode(err error, code ErrorCode) bool {
	var domainErr *DomainError
	return errors.As(err, &domainErr) && domainErr.Code == code
}

func validation(field, message string) error {
	return &DomainError{Code: CodeValidation, Field: field, Message: message}
}

func conflict(message string, cause error) error {
	return &DomainError{Code: CodeConflict, Message: message, Cause: cause}
}

func unavailable(message string, cause error) error {
	return &DomainError{Code: CodeUnavailable, Message: message, Cause: cause}
}
