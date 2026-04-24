package apperrors

import "net/http"

// AppError is a typed error that carries an HTTP status code.
// Services return these so handlers can respond with the correct status
// without manual code-to-status mapping at every call site.
type AppError struct {
	Code    int
	Message string
}

func (e *AppError) Error() string { return e.Message }

func NewBadRequestError(msg string) *AppError {
	return &AppError{Code: http.StatusBadRequest, Message: msg}
}

func NewValidationError(msg string) *AppError {
	return &AppError{Code: http.StatusBadRequest, Message: msg}
}

func NewNotFoundError(msg string) *AppError {
	return &AppError{Code: http.StatusNotFound, Message: msg}
}

func NewConflictError(msg string) *AppError {
	return &AppError{Code: http.StatusConflict, Message: msg}
}

func NewUnauthorizedError(msg string) *AppError {
	return &AppError{Code: http.StatusUnauthorized, Message: msg}
}

func NewInternalError(msg string) *AppError {
	return &AppError{Code: http.StatusInternalServerError, Message: msg}
}
