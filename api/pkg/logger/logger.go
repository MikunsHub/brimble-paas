package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type responseBodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *responseBodyWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func Initialize(env string) *slog.Logger {
	var opts slog.HandlerOptions

	switch env {
	case "production", "staging":
		opts.Level = slog.LevelInfo
	default:
		opts.Level = slog.LevelDebug
	}

	opts.ReplaceAttr = func(groups []string, a slog.Attr) slog.Attr {
		if a.Key == slog.LevelKey {
			level := a.Value.Any().(slog.Level)
			switch level {
			case slog.LevelDebug:
				return slog.String("log_level", "DEBUG")
			case slog.LevelInfo:
				return slog.String("log_level", "INFO")
			case slog.LevelWarn:
				return slog.String("log_level", "WARN")
			case slog.LevelError:
				return slog.String("log_level", "ERROR")
			default:
				return slog.String("log_level", "INFO")
			}
		}
		return a
	}

	handler := slog.NewJSONHandler(os.Stdout, &opts)
	logger := slog.New(handler).With(slog.String("app", "brimble-paas"))
	slog.SetDefault(logger)

	return logger
}

func getCallerInfo() (file string, line int, funcName string, found bool) {
	var pc uintptr

	for i := 1; i < 10; i++ {
		var ok bool
		pc, file, line, ok = runtime.Caller(i)
		if !ok {
			break
		}

		fn := runtime.FuncForPC(pc)
		if fn == nil {
			continue
		}

		if strings.Contains(fn.Name(), "pkg/logger") ||
			strings.Contains(fn.Name(), "github.com/gin-gonic/gin") {
			continue
		}

		if idx := strings.Index(file, "brimble-paas/"); idx != -1 {
			file = file[idx+len("brimble-paas/"):]
		}

		if idx := strings.Index(fn.Name(), "brimble-paas/"); idx != -1 {
			funcName = fn.Name()[idx+len("brimble-paas/"):]
		} else {
			funcName = fn.Name()
		}

		found = true
		break
	}

	return file, line, funcName, found
}

func GinMiddleware(logger *slog.Logger, environment string) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery
		method := c.Request.Method

		if path == "/health" {
			c.Next()
			return
		}

		if method == "POST" || method == "PATCH" || method == "PUT" {
			if c.Request.Body != nil {
				bodyBytes, err := io.ReadAll(c.Request.Body)
				if err == nil {
					c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
				}
			}
		}

		bodyWriter := &responseBodyWriter{
			ResponseWriter: c.Writer,
			body:           bytes.NewBufferString(""),
		}
		c.Writer = bodyWriter

		c.Next()

		latency := time.Since(start)
		statusCode := c.Writer.Status()
		errorMessage := c.Errors.String()

		attrs := []slog.Attr{
			slog.String("method", method),
			slog.String("path", path),
			slog.String("query", query),
			slog.Int("status_code", statusCode),
			slog.Float64("latency_ms", float64(latency.Milliseconds())),
			slog.String("environment", environment),
		}

		if xSource := c.Request.Header.Get("X-Source"); xSource != "" {
			attrs = append(attrs, slog.String("x_source", xSource))
		}

		if c.Request.Header != nil {
			attrs = append(attrs, slog.Any("headers", redactHeaders(c.Request.Header)))
		}

		if errorMessage != "" {
			attrs = append(attrs, slog.String("error", errorMessage))
		}

		if statusCode >= 400 {
			responseBody := bodyWriter.body.String()
			if responseBody != "" {
				var apiResponse map[string]interface{}
				if err := json.Unmarshal([]byte(responseBody), &apiResponse); err == nil {
					if message, exists := apiResponse["message"]; exists {
						if msgStr, ok := message.(string); ok && msgStr != "" {
							attrs = append(attrs, slog.String("error_message", msgStr))
						}
					}
				}
			}
		}

		var level slog.Level
		switch {
		case statusCode >= 500:
			level = slog.LevelError
		case statusCode >= 400:
			level = slog.LevelWarn
		default:
			level = slog.LevelInfo
		}

		logger.LogAttrs(c.Request.Context(), level, "HTTP Request", attrs...)
	}
}

func Info(msg string, args ...any) {
	file, line, funcName, found := getCallerInfo()

	attrs := convertArgsToAttrs(args...)
	if found {
		attrs = append([]slog.Attr{
			slog.Group("source",
				slog.String("function", funcName),
				slog.String("file", file),
				slog.Int("line", line),
			),
		}, attrs...)
	}

	slog.Default().LogAttrs(context.Background(), slog.LevelInfo, msg, attrs...)
}

func Error(err error, msg string, args ...any) {
	file, line, funcName, found := getCallerInfo()

	attrs := convertArgsToAttrs(args...)
	attrs = append(attrs, slog.Any("error", err))
	if found {
		attrs = append([]slog.Attr{
			slog.Group("source",
				slog.String("function", funcName),
				slog.String("file", file),
				slog.Int("line", line),
			),
		}, attrs...)
	}

	slog.Default().LogAttrs(context.Background(), slog.LevelError, msg, attrs...)
}

func Debug(msg string, args ...any) {
	file, line, funcName, found := getCallerInfo()

	attrs := convertArgsToAttrs(args...)
	if found {
		attrs = append([]slog.Attr{
			slog.Group("source",
				slog.String("function", funcName),
				slog.String("file", file),
				slog.Int("line", line),
			),
		}, attrs...)
	}

	slog.Default().LogAttrs(context.Background(), slog.LevelDebug, msg, attrs...)
}

func Warn(msg string, args ...any) {
	file, line, funcName, found := getCallerInfo()

	attrs := convertArgsToAttrs(args...)
	if found {
		attrs = append([]slog.Attr{
			slog.Group("source",
				slog.String("function", funcName),
				slog.String("file", file),
				slog.Int("line", line),
			),
		}, attrs...)
	}

	slog.Default().LogAttrs(context.Background(), slog.LevelWarn, msg, attrs...)
}

var sensitiveHeaders = map[string]bool{
	"authorization": true,
	"cookie":        true,
	"set-cookie":    true,
}

func redactHeaders(headers map[string][]string) map[string]interface{} {
	redacted := make(map[string]interface{})
	for key, values := range headers {
		if sensitiveHeaders[strings.ToLower(key)] {
			redacted[key] = "REDACTED"
		} else if len(values) == 1 {
			redacted[key] = values[0]
		} else {
			redacted[key] = values
		}
	}
	return redacted
}

func convertArgsToAttrs(args ...any) []slog.Attr {
	var attrs []slog.Attr

	for i := 0; i < len(args); i++ {
		if attr, ok := args[i].(slog.Attr); ok {
			attrs = append(attrs, attr)
			continue
		}

		if i+1 < len(args) {
			if key, ok := args[i].(string); ok {
				attrs = append(attrs, slog.Any(key, args[i+1]))
				i++
				continue
			}
		}
	}

	return attrs
}
