package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/brimble/paas/pkg/responses"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"gorm.io/gorm"
)

type BaseHandler struct {
	DB     *gorm.DB
	Logger *slog.Logger
}

func (h *BaseHandler) OK(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, responses.NewApiResponse(message, data))
}

func (h *BaseHandler) Created(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusCreated, responses.NewApiResponse(message, data))
}

func (h *BaseHandler) OKPaginated(c *gin.Context, message string, data interface{}, meta responses.Meta) {
	c.JSON(http.StatusOK, responses.NewApiResponsePaginated(message, data, meta))
}

func (h *BaseHandler) BadRequest(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, responses.NewApiResponse(message, nil))
}

func (h *BaseHandler) NotFound(c *gin.Context, message string) {
	c.JSON(http.StatusNotFound, responses.NewApiResponse(message, nil))
}

func (h *BaseHandler) InternalError(c *gin.Context, message string) {
	c.JSON(http.StatusInternalServerError, responses.NewApiResponse(message, nil))
}

func (h *BaseHandler) BindJSON(c *gin.Context, req interface{}) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		h.BadRequest(c, formatBindError(err))
		return false
	}
	return true
}


func formatBindError(err error) string {
	if valErrs, ok := err.(validator.ValidationErrors); ok {
		return formatValidationErrors(valErrs)
	}
	if jsonErr, ok := err.(*json.UnmarshalTypeError); ok {
		field := lastSegment(jsonErr.Field, ".")
		return fmt.Sprintf("%s expected %s but got %s", friendlyField(field), jsonErr.Type, jsonErr.Value)
	}
	if _, ok := err.(*json.SyntaxError); ok {
		return "invalid JSON in request body"
	}
	return err.Error()
}

func formatValidationErrors(errs validator.ValidationErrors) string {
	for _, e := range errs {
		field := friendlyField(e.Field())
		switch e.Tag() {
		case "required":
			return fmt.Sprintf("'%s' is required", field)
		case "email":
			return "invalid email address"
		case "min":
			if e.Kind().String() == "string" {
				return fmt.Sprintf("%s must be at least %s characters", field, e.Param())
			}
			return fmt.Sprintf("%s must be at least %s", field, e.Param())
		case "max":
			if e.Kind().String() == "string" {
				return fmt.Sprintf("%s must not exceed %s characters", field, e.Param())
			}
			return fmt.Sprintf("%s must not exceed %s", field, e.Param())
		case "oneof":
			return fmt.Sprintf("%s must be one of: %s", field, e.Param())
		default:
			return fmt.Sprintf("invalid value for %s", field)
		}
	}
	return "invalid input"
}

func friendlyField(field string) string {
	var b strings.Builder
	for i, r := range field {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}

func lastSegment(s, sep string) string {
	parts := strings.Split(s, sep)
	return parts[len(parts)-1]
}
