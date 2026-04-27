package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestResponseBodyWriter_Write(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	writer := &responseBodyWriter{
		ResponseWriter: c.Writer,
		body:           bytes.NewBuffer(nil),
	}

	n, err := writer.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", writer.body.String())
}

func TestInitialize(t *testing.T) {
	for _, tc := range []struct {
		name          string
		env           string
		logFn         func(*slog.Logger)
		expectedLevel string
	}{
		{name: "production info level", env: "production", logFn: func(l *slog.Logger) { l.Info("hello") }, expectedLevel: "INFO"},
		{name: "development debug level", env: "development", logFn: func(l *slog.Logger) { l.Debug("hello") }, expectedLevel: "DEBUG"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			origStdout := os.Stdout
			r, w, err := os.Pipe()
			require.NoError(t, err)
			os.Stdout = w
			defer func() { os.Stdout = origStdout }()

			logger := Initialize(tc.env)
			tc.logFn(logger)
			require.NoError(t, w.Close())

			out, err := io.ReadAll(r)
			require.NoError(t, err)
			assert.Contains(t, string(out), `"app":"brimble-paas"`)
			assert.Contains(t, string(out), `"log_level":"`+tc.expectedLevel+`"`)
		})
	}
}

func TestLogHelpers(t *testing.T) {
	handler := &captureHandler{}
	orig := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(orig)

	Info("info", "id", 1)
	Error(assert.AnError, "error", "id", 2)
	Debug("debug", "id", 3)
	Warn("warn", "id", 4)

	require.Len(t, handler.records, 4)
	assert.Equal(t, "info", handler.records[0].Message)
	assert.Equal(t, slog.LevelInfo, handler.records[0].Level)
	assert.Equal(t, "error", handler.records[1].Message)
	assert.Equal(t, slog.LevelError, handler.records[1].Level)
	assert.Equal(t, "debug", handler.records[2].Message)
	assert.Equal(t, slog.LevelDebug, handler.records[2].Level)
	assert.Equal(t, "warn", handler.records[3].Message)
	assert.Equal(t, slog.LevelWarn, handler.records[3].Level)
}

func TestGinMiddleware(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := &captureHandler{}
	logger := slog.New(handler)

	t.Run("logs request details", func(t *testing.T) {
		r := gin.New()
		r.Use(GinMiddleware(logger, "test"))
		r.POST("/items", func(c *gin.Context) {
			c.JSON(http.StatusBadRequest, gin.H{"message": "bad input"})
		})

		req := httptest.NewRequest(http.MethodPost, "/items?q=1", bytes.NewBufferString(`{"foo":"bar"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer secret")
		req.Header.Set("X-Source", "frontend")
		rec := httptest.NewRecorder()

		r.ServeHTTP(rec, req)

		require.NotEmpty(t, handler.records)
		last := handler.records[len(handler.records)-1]
		assert.Equal(t, slog.LevelWarn, last.Level)
		assert.Equal(t, "HTTP Request", last.Message)
		assert.Equal(t, "POST", last.Attrs["method"])
		assert.Equal(t, "/items", last.Attrs["path"])
		assert.Equal(t, float64(http.StatusBadRequest), last.Attrs["status_code"])
		assert.Equal(t, "frontend", last.Attrs["x_source"])
		assert.Equal(t, "test", last.Attrs["environment"])
		assert.Equal(t, "bad input", last.Attrs["error_message"])

		headers, ok := last.Attrs["headers"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "REDACTED", headers["Authorization"])
	})

	t.Run("skips health path", func(t *testing.T) {
		before := len(handler.records)

		r := gin.New()
		r.Use(GinMiddleware(logger, "test"))
		r.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Len(t, handler.records, before)
	})
}

type captureHandler struct {
	mu      sync.Mutex
	records []capturedRecord
}

type capturedRecord struct {
	Level   slog.Level
	Message string
	Attrs   map[string]any
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	rec := capturedRecord{
		Level:   r.Level,
		Message: r.Message,
		Attrs:   map[string]any{},
	}
	r.Attrs(func(a slog.Attr) bool {
		rec.Attrs[a.Key] = attrValue(a.Value)
		return true
	})

	h.mu.Lock()
	h.records = append(h.records, rec)
	h.mu.Unlock()
	return nil
}

func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func attrValue(v slog.Value) any {
	switch v.Kind() {
	case slog.KindAny:
		return v.Any()
	case slog.KindBool:
		return v.Bool()
	case slog.KindDuration:
		return v.Duration()
	case slog.KindFloat64:
		return v.Float64()
	case slog.KindInt64:
		return float64(v.Int64())
	case slog.KindString:
		return v.String()
	case slog.KindTime:
		return v.Time()
	case slog.KindUint64:
		return float64(v.Uint64())
	case slog.KindGroup:
		group := map[string]any{}
		for _, a := range v.Group() {
			group[a.Key] = attrValue(a.Value)
		}
		return group
	default:
		var out any
		_ = json.Unmarshal([]byte(v.String()), &out)
		if out != nil {
			return out
		}
		return v.String()
	}
}
