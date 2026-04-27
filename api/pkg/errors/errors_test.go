package apperrors

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewBadRequestError(t *testing.T) {
	t.Parallel()
	err := NewBadRequestError("bad")
	assert.Equal(t, http.StatusBadRequest, err.Code)
	assert.Equal(t, "bad", err.Message)
}

func TestNewValidationError(t *testing.T) {
	t.Parallel()
	err := NewValidationError("invalid")
	assert.Equal(t, http.StatusBadRequest, err.Code)
	assert.Equal(t, "invalid", err.Message)
}

func TestNewNotFoundError(t *testing.T) {
	t.Parallel()
	err := NewNotFoundError("missing")
	assert.Equal(t, http.StatusNotFound, err.Code)
	assert.Equal(t, "missing", err.Message)
}

func TestNewConflictError(t *testing.T) {
	t.Parallel()
	err := NewConflictError("conflict")
	assert.Equal(t, http.StatusConflict, err.Code)
	assert.Equal(t, "conflict", err.Message)
}

func TestNewUnauthorizedError(t *testing.T) {
	t.Parallel()
	err := NewUnauthorizedError("unauthorized")
	assert.Equal(t, http.StatusUnauthorized, err.Code)
	assert.Equal(t, "unauthorized", err.Message)
}

func TestNewInternalError(t *testing.T) {
	t.Parallel()
	err := NewInternalError("internal")
	assert.Equal(t, http.StatusInternalServerError, err.Code)
	assert.Equal(t, "internal", err.Message)
}
