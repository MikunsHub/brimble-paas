package logger

import (
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactHeaders(t *testing.T) {
	t.Parallel()

	headers := http.Header{
		"Authorization": []string{"Bearer token"},
		"Cookie":        []string{"a=b"},
		"X-Trace":       []string{"trace-1"},
		"X-Multi":       []string{"a", "b"},
	}

	redacted := redactHeaders(headers)
	assert.Equal(t, "REDACTED", redacted["Authorization"])
	assert.Equal(t, "REDACTED", redacted["Cookie"])
	assert.Equal(t, "trace-1", redacted["X-Trace"])
	assert.Equal(t, []string{"a", "b"}, redacted["X-Multi"])
}

func TestConvertArgsToAttrs(t *testing.T) {
	t.Parallel()

	attrs := convertArgsToAttrs(
		"id", 1,
		slog.String("env", "test"),
		"orphan",
		123,
	)

	assert.Len(t, attrs, 3)
	assert.Equal(t, "id", attrs[0].Key)
	assert.Equal(t, int64(1), attrs[0].Value.Int64())
	assert.Equal(t, "env", attrs[1].Key)
	assert.Equal(t, "test", attrs[1].Value.String())
	assert.Equal(t, "orphan", attrs[2].Key)
	assert.Equal(t, int64(123), attrs[2].Value.Int64())
}
