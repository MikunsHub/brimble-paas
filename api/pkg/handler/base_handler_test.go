package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	apperrors "github.com/brimble/paas/pkg/errors"
	"github.com/brimble/paas/pkg/responses"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBaseHandler_BindJSON(t *testing.T) {
	t.Parallel()

	type request struct {
		Name string `json:"name" binding:"required"`
		Age  int    `json:"age"`
	}

	tests := []struct {
		name       string
		body       string
		wantOK     bool
		wantStatus int
		wantMsg    string
	}{
		{name: "valid json", body: `{"name":"Ada","age":3}`, wantOK: true},
		{name: "invalid json syntax", body: `{"name":"Ada"`, wantStatus: http.StatusBadRequest, wantMsg: "invalid JSON in request body"},
		{name: "validation error", body: `{"age":3}`, wantStatus: http.StatusBadRequest, wantMsg: "'name' is required"},
		{name: "type mismatch", body: `{"name":"Ada","age":"old"}`, wantStatus: http.StatusBadRequest, wantMsg: "age expected int but got string"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestBaseHandler()
			c, rec := newJSONContext(tc.body)
			var req request

			ok := h.BindJSON(c, &req)

			assert.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				assert.Equal(t, "Ada", req.Name)
				return
			}

			assert.Equal(t, tc.wantStatus, rec.Code)
			assert.Equal(t, tc.wantMsg, decodeBaseResponse(t, rec.Body.Bytes()).Message)
		})
	}
}

func TestBaseHandler_HandleErr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantMsg    string
	}{
		{name: "bad request", err: apperrors.NewBadRequestError("bad request"), wantStatus: http.StatusBadRequest, wantMsg: "bad request"},
		{name: "not found", err: apperrors.NewNotFoundError("missing"), wantStatus: http.StatusNotFound, wantMsg: "missing"},
		{name: "internal", err: apperrors.NewInternalError("broken"), wantStatus: http.StatusInternalServerError, wantMsg: "broken"},
		{name: "generic error", err: errors.New("boom"), wantStatus: http.StatusInternalServerError, wantMsg: "internal server error"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestBaseHandler()
			c, rec := newTestContext()

			h.HandleErr(c, tc.err)

			assert.Equal(t, tc.wantStatus, rec.Code)
			assert.Equal(t, tc.wantMsg, decodeBaseResponse(t, rec.Body.Bytes()).Message)
		})
	}
}

func TestBaseHandler_OK(t *testing.T) {
	t.Parallel()

	h := newTestBaseHandler()
	c, rec := newTestContext()
	h.OK(c, "ok", gin.H{"id": 1})

	assert.Equal(t, http.StatusOK, rec.Code)
	resp := decodeBaseResponse(t, rec.Body.Bytes())
	assert.Equal(t, "ok", resp.Message)
}

func TestBaseHandler_Created(t *testing.T) {
	t.Parallel()

	h := newTestBaseHandler()
	c, rec := newTestContext()
	h.Created(c, "created", gin.H{"id": 1})

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "created", decodeBaseResponse(t, rec.Body.Bytes()).Message)
}

func TestBaseHandler_OKPaginated(t *testing.T) {
	t.Parallel()

	h := newTestBaseHandler()
	c, rec := newTestContext()
	h.OKPaginated(c, "ok", []string{"a", "b"}, responses.Meta{TotalItems: 2, ItemCount: 2, ItemsPerPage: 2, TotalPages: 1, CurrentPage: 1})

	assert.Equal(t, http.StatusOK, rec.Code)

	var payload struct {
		Message string         `json:"message"`
		Data    []string       `json:"data"`
		Meta    responses.Meta `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	assert.Equal(t, "ok", payload.Message)
	assert.Equal(t, int64(2), payload.Meta.TotalItems)
}

func TestBaseHandler_BadRequest(t *testing.T) {
	t.Parallel()
	h := newTestBaseHandler()
	c, rec := newTestContext()
	h.BadRequest(c, "bad")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestBaseHandler_NotFound(t *testing.T) {
	t.Parallel()
	h := newTestBaseHandler()
	c, rec := newTestContext()
	h.NotFound(c, "missing")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestBaseHandler_InternalError(t *testing.T) {
	t.Parallel()
	h := newTestBaseHandler()
	c, rec := newTestContext()
	h.InternalError(c, "broken")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestFormatBindError(t *testing.T) {
	t.Parallel()

	type request struct {
		Email string `validate:"required,email"`
	}

	validate := validator.New()
	err := validate.Struct(request{})
	require.Error(t, err)

	valErrs, ok := err.(validator.ValidationErrors)
	require.True(t, ok)
	assert.Equal(t, "'email' is required", formatBindError(valErrs))

	typeMismatch := &json.UnmarshalTypeError{Field: "payload.Age", Type: reflect.TypeFor[int](), Value: "string"}
	assert.Equal(t, "age expected int but got string", formatBindError(typeMismatch))

	syntaxErr := &json.SyntaxError{}
	assert.Equal(t, "invalid JSON in request body", formatBindError(syntaxErr))

	assert.Equal(t, "boom", formatBindError(errors.New("boom")))
}

func TestFormatValidationErrors(t *testing.T) {
	t.Parallel()

	type validationCase struct {
		name   string
		value  any
		expect string
	}

	tests := []validationCase{
		{name: "required", value: struct {
			Name string `validate:"required"`
		}{}, expect: "'name' is required"},
		{name: "email", value: struct {
			Email string `validate:"email"`
		}{Email: "bad"}, expect: "invalid email address"},
		{name: "min string", value: struct {
			Name string `validate:"min=3"`
		}{Name: "go"}, expect: "name must be at least 3 characters"},
		{name: "max string", value: struct {
			Name string `validate:"max=3"`
		}{Name: "gopher"}, expect: "name must not exceed 3 characters"},
		{name: "oneof", value: struct {
			Env string `validate:"oneof=dev prod"`
		}{Env: "test"}, expect: "env must be one of: dev prod"},
		{name: "default tag", value: struct {
			Code string `validate:"startswith=abc"`
		}{Code: "xyz"}, expect: "invalid value for code"},
	}

	validate := validator.New()
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validate.Struct(tc.value)
			require.Error(t, err)
			valErrs, ok := err.(validator.ValidationErrors)
			require.True(t, ok)
			assert.Equal(t, tc.expect, formatValidationErrors(valErrs))
		})
	}
}

func TestFriendlyField(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "git url", friendlyField("GitURL"))
	assert.Equal(t, "start cmd", friendlyField("StartCmd"))
}

func TestLastSegment(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "Field", lastSegment("payload.inner.Field", "."))
	assert.Equal(t, "field", lastSegment("field", "."))
}

func newTestBaseHandler() *BaseHandler {
	gin.SetMode(gin.TestMode)
	return &BaseHandler{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func newTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	return c, rec
}

func newJSONContext(body string) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, rec
}

type baseResponse struct {
	Message string `json:"message"`
}

func decodeBaseResponse(t *testing.T, body []byte) baseResponse {
	t.Helper()
	var resp baseResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	return resp
}
