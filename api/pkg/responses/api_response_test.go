package responses

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewApiResponse(t *testing.T) {
	t.Parallel()

	resp := NewApiResponse("ok", map[string]any{"id": 1})
	assert.Equal(t, "ok", resp.Message)
	assert.Equal(t, map[string]any{"id": 1}, resp.Data)
}

func TestNewApiResponsePaginated(t *testing.T) {
	t.Parallel()

	meta := Meta{TotalItems: 10, ItemCount: 2, ItemsPerPage: 2, TotalPages: 5, CurrentPage: 1}
	resp := NewApiResponsePaginated("ok", []string{"a", "b"}, meta)
	assert.Equal(t, "ok", resp.Message)
	assert.Equal(t, meta, resp.Meta)
}
