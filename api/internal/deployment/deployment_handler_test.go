package deployment

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/brimble/paas/entities"
	apperrors "github.com/brimble/paas/pkg/errors"
	"github.com/brimble/paas/pkg/handler"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_Create(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	deployment := &entities.Deployment{ID: "dep_1", Subdomain: "demo", Status: entities.StatusPending, CreatedAt: now}

	tests := []struct {
		name       string
		body       string
		service    *mockService
		wantStatus int
		wantMsg    string
	}{
		{
			name: "valid git url",
			body: `{"git_url":"https://github.com/brimble/app.git"}`,
			service: &mockService{createFunc: func(_ context.Context, req CreateDeploymentRequest) (*entities.Deployment, error) {
				require.NotNil(t, req.GitURL)
				assert.Equal(t, "https://github.com/brimble/app.git", *req.GitURL)
				return deployment, nil
			}},
			wantStatus: http.StatusCreated,
			wantMsg:    "deployment created",
		},
		{
			name: "valid file path",
			body: `{"file_path":"uploads/app.zip"}`,
			service: &mockService{createFunc: func(_ context.Context, req CreateDeploymentRequest) (*entities.Deployment, error) {
				require.NotNil(t, req.FilePath)
				assert.Equal(t, "uploads/app.zip", *req.FilePath)
				return deployment, nil
			}},
			wantStatus: http.StatusCreated,
			wantMsg:    "deployment created",
		},
		{
			name: "neither provided",
			body: `{}`,
			service: &mockService{createFunc: func(_ context.Context, _ CreateDeploymentRequest) (*entities.Deployment, error) {
				return nil, apperrors.NewBadRequestError("provide exactly one of git_url or file_path")
			}},
			wantStatus: http.StatusBadRequest,
			wantMsg:    "provide exactly one of git_url or file_path",
		},
		{
			name: "both provided",
			body: `{"git_url":"https://github.com/brimble/app.git","file_path":"uploads/app.zip"}`,
			service: &mockService{createFunc: func(_ context.Context, _ CreateDeploymentRequest) (*entities.Deployment, error) {
				return nil, apperrors.NewBadRequestError("provide exactly one of git_url or file_path")
			}},
			wantStatus: http.StatusBadRequest,
			wantMsg:    "provide exactly one of git_url or file_path",
		},
		{
			name: "file missing on s3",
			body: `{"file_path":"uploads/missing.zip"}`,
			service: &mockService{createFunc: func(_ context.Context, _ CreateDeploymentRequest) (*entities.Deployment, error) {
				return nil, apperrors.NewBadRequestError("file_path does not exist")
			}},
			wantStatus: http.StatusBadRequest,
			wantMsg:    "file_path does not exist",
		},
		{
			name:       "invalid json",
			body:       `{"git_url":`,
			service:    &mockService{},
			wantStatus: http.StatusBadRequest,
			wantMsg:    "invalid JSON in request body",
		},
		{
			name: "generic service error",
			body: `{"git_url":"https://github.com/brimble/app.git"}`,
			service: &mockService{createFunc: func(_ context.Context, _ CreateDeploymentRequest) (*entities.Deployment, error) {
				return nil, assert.AnError
			}},
			wantStatus: http.StatusInternalServerError,
			wantMsg:    "internal server error",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			router, _ := setupHandler(tc.service)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/deployments", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code)
			assert.Equal(t, tc.wantMsg, decodeAPIResponse(t, rec.Body.Bytes()).Message)
		})
	}
}

func TestHandler_CreateUploadURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		service    *mockService
		wantStatus int
		wantMsg    string
	}{
		{
			name: "valid request",
			body: `{"file_name":"app.zip","content_type":"application/zip"}`,
			service: &mockService{createUploadURLFunc: func(_ context.Context, req CreateUploadURLRequest) (*CreateUploadURLResponse, error) {
				assert.Equal(t, "app.zip", req.FileName)
				return &CreateUploadURLResponse{FilePath: "uploads/app.zip", URL: "https://example.com", Method: "PUT"}, nil
			}},
			wantStatus: http.StatusCreated,
			wantMsg:    "upload url created",
		},
		{
			name: "empty filename",
			body: `{"file_name":"","content_type":"application/zip"}`,
			service: &mockService{createUploadURLFunc: func(_ context.Context, _ CreateUploadURLRequest) (*CreateUploadURLResponse, error) {
				return nil, apperrors.NewBadRequestError("file_name is required")
			}},
			wantStatus: http.StatusBadRequest,
			wantMsg:    "file_name is required",
		},
		{
			name: "s3 error",
			body: `{"file_name":"app.zip","content_type":"application/zip"}`,
			service: &mockService{createUploadURLFunc: func(_ context.Context, _ CreateUploadURLRequest) (*CreateUploadURLResponse, error) {
				return nil, apperrors.NewInternalError("failed to create upload url")
			}},
			wantStatus: http.StatusInternalServerError,
			wantMsg:    "failed to create upload url",
		},
		{
			name: "zip validation error",
			body: `{"file_name":"app.txt","content_type":"text/plain"}`,
			service: &mockService{createUploadURLFunc: func(_ context.Context, _ CreateUploadURLRequest) (*CreateUploadURLResponse, error) {
				return nil, apperrors.NewBadRequestError("file must be a .zip archive")
			}},
			wantStatus: http.StatusBadRequest,
			wantMsg:    "file must be a .zip archive",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			router, _ := setupHandler(tc.service)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/deployments/upload-url", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code)
			assert.Equal(t, tc.wantMsg, decodeAPIResponse(t, rec.Body.Bytes()).Message)
		})
	}
}

func TestHandler_List(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		service    *mockService
		wantStatus int
		wantCount  int
		wantMsg    string
	}{
		{
			name: "success",
			service: &mockService{listFunc: func(context.Context) ([]*entities.Deployment, error) {
				return []*entities.Deployment{{ID: "1", Subdomain: "one", Status: entities.StatusRunning}, {ID: "2", Subdomain: "two", Status: entities.StatusStopped}}, nil
			}},
			wantStatus: http.StatusOK,
			wantCount:  2,
			wantMsg:    "deployments fetched",
		},
		{
			name:       "empty list",
			service:    &mockService{listFunc: func(context.Context) ([]*entities.Deployment, error) { return []*entities.Deployment{}, nil }},
			wantStatus: http.StatusOK,
			wantCount:  0,
			wantMsg:    "deployments fetched",
		},
		{
			name: "service error",
			service: &mockService{listFunc: func(context.Context) ([]*entities.Deployment, error) {
				return nil, apperrors.NewInternalError("failed to list deployments")
			}},
			wantStatus: http.StatusInternalServerError,
			wantCount:  0,
			wantMsg:    "failed to list deployments",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			router, _ := setupHandler(tc.service)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/deployments", nil)

			router.ServeHTTP(rec, req)

			resp := decodeAPIResponse(t, rec.Body.Bytes())
			assert.Equal(t, tc.wantStatus, rec.Code)
			assert.Equal(t, tc.wantMsg, resp.Message)
			if rec.Code == http.StatusOK {
				items := resp.Data.([]any)
				assert.Len(t, items, tc.wantCount)
			}
		})
	}
}

func TestHandler_Get(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		service    *mockService
		wantStatus int
		wantMsg    string
	}{
		{
			name: "found",
			service: &mockService{getFunc: func(context.Context, string) (*entities.Deployment, error) {
				return &entities.Deployment{ID: "dep_1", Subdomain: "app", Status: entities.StatusRunning}, nil
			}},
			wantStatus: http.StatusOK,
			wantMsg:    "deployment fetched",
		},
		{
			name: "not found",
			service: &mockService{getFunc: func(context.Context, string) (*entities.Deployment, error) {
				return nil, apperrors.NewNotFoundError("deployment not found")
			}},
			wantStatus: http.StatusNotFound,
			wantMsg:    "deployment not found",
		},
		{
			name: "service error",
			service: &mockService{getFunc: func(context.Context, string) (*entities.Deployment, error) {
				return nil, apperrors.NewInternalError("failed to fetch deployment")
			}},
			wantStatus: http.StatusInternalServerError,
			wantMsg:    "failed to fetch deployment",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			router, _ := setupHandler(tc.service)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/deployments/dep_1", nil)

			router.ServeHTTP(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code)
			assert.Equal(t, tc.wantMsg, decodeAPIResponse(t, rec.Body.Bytes()).Message)
		})
	}
}

func TestHandler_Delete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		service    *mockService
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "success",
			service:    &mockService{teardownFunc: func(context.Context, string) error { return nil }},
			wantStatus: http.StatusOK,
			wantMsg:    "deployment stopped",
		},
		{
			name:       "not found",
			service:    &mockService{teardownFunc: func(context.Context, string) error { return apperrors.NewNotFoundError("deployment not found") }},
			wantStatus: http.StatusNotFound,
			wantMsg:    "deployment not found",
		},
		{
			name: "service error",
			service: &mockService{teardownFunc: func(context.Context, string) error {
				return apperrors.NewInternalError("failed to update deployment status")
			}},
			wantStatus: http.StatusInternalServerError,
			wantMsg:    "failed to update deployment status",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			router, _ := setupHandler(tc.service)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodDelete, "/api/deployments/dep_1", nil)

			router.ServeHTTP(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code)
			assert.Equal(t, tc.wantMsg, decodeAPIResponse(t, rec.Body.Bytes()).Message)
		})
	}
}

func TestHandler_Restart(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		service    *mockService
		wantStatus int
		wantMsg    string
	}{
		{
			name: "success",
			service: &mockService{restartFunc: func(context.Context, string) (*entities.Deployment, error) {
				return &entities.Deployment{ID: "dep_1", Subdomain: "app", Status: entities.StatusPending}, nil
			}},
			wantStatus: http.StatusOK,
			wantMsg:    "deployment restarted",
		},
		{
			name: "not restartable state",
			service: &mockService{restartFunc: func(context.Context, string) (*entities.Deployment, error) {
				return nil, apperrors.NewBadRequestError("deployment is not in a restartable state")
			}},
			wantStatus: http.StatusBadRequest,
			wantMsg:    "deployment is not in a restartable state",
		},
		{
			name: "no source",
			service: &mockService{restartFunc: func(context.Context, string) (*entities.Deployment, error) {
				return nil, apperrors.NewBadRequestError("no source available to restart from")
			}},
			wantStatus: http.StatusBadRequest,
			wantMsg:    "no source available to restart from",
		},
		{
			name: "not found",
			service: &mockService{restartFunc: func(context.Context, string) (*entities.Deployment, error) {
				return nil, apperrors.NewNotFoundError("deployment not found")
			}},
			wantStatus: http.StatusNotFound,
			wantMsg:    "deployment not found",
		},
		{
			name: "service error",
			service: &mockService{restartFunc: func(context.Context, string) (*entities.Deployment, error) {
				return nil, apperrors.NewInternalError("failed to reset deployment")
			}},
			wantStatus: http.StatusInternalServerError,
			wantMsg:    "failed to reset deployment",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			router, _ := setupHandler(tc.service)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/deployments/dep_1/restart", nil)

			router.ServeHTTP(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code)
			assert.Equal(t, tc.wantMsg, decodeAPIResponse(t, rec.Body.Bytes()).Message)
		})
	}
}

func TestHandler_GetLogs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		query          string
		service        *mockService
		wantStatus     int
		wantMsg        string
		wantOffset     int
		wantLogEntries int
	}{
		{
			name:       "success with offset",
			query:      "?offset=3",
			wantOffset: 3,
			service: &mockService{getLogsFunc: func(_ context.Context, _ string, offset int) ([]*entities.DeploymentLog, error) {
				assert.Equal(t, 3, offset)
				return []*entities.DeploymentLog{{ID: "log_1", DeploymentID: "dep_1", Stream: "stdout", Phase: "build", Content: "ready", Timestamp: time.Now()}}, nil
			}},
			wantStatus:     http.StatusOK,
			wantMsg:        "logs fetched",
			wantLogEntries: 1,
		},
		{
			name:       "default offset",
			query:      "",
			wantOffset: 0,
			service: &mockService{getLogsFunc: func(_ context.Context, _ string, offset int) ([]*entities.DeploymentLog, error) {
				assert.Equal(t, 0, offset)
				return []*entities.DeploymentLog{}, nil
			}},
			wantStatus:     http.StatusOK,
			wantMsg:        "logs fetched",
			wantLogEntries: 0,
		},
		{
			name:       "service error",
			query:      "",
			wantOffset: 0,
			service: &mockService{getLogsFunc: func(context.Context, string, int) ([]*entities.DeploymentLog, error) {
				return nil, apperrors.NewInternalError("failed to fetch logs")
			}},
			wantStatus: http.StatusInternalServerError,
			wantMsg:    "failed to fetch logs",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			router, _ := setupHandler(tc.service)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/deployments/dep_1/logs"+tc.query, nil)

			router.ServeHTTP(rec, req)

			resp := decodeAPIResponse(t, rec.Body.Bytes())
			assert.Equal(t, tc.wantStatus, rec.Code)
			assert.Equal(t, tc.wantMsg, resp.Message)
			if rec.Code == http.StatusOK {
				assert.Len(t, resp.Data.([]any), tc.wantLogEntries)
			}
		})
	}
}

func TestHandler_StreamLogs(t *testing.T) {
	t.Parallel()

	history := []StreamLogEvent{{Index: 0, Stream: "stdout", Phase: "build", Content: "history"}}
	liveLogs := make(chan StreamLogEvent, 1)
	liveLogs <- StreamLogEvent{Index: 1, Stream: "stdout", Phase: "runtime", Content: "live"}
	close(liveLogs)
	statusUpdates := make(chan StreamStatusEvent, 1)
	statusUpdates <- StreamStatusEvent{Status: entities.StatusRunning, LiveURL: "http://demo.example.com"}
	close(statusUpdates)

	tests := []struct {
		name       string
		query      string
		service    *mockService
		wantStatus int
		wantBody   []string
		wantMsg    string
	}{
		{
			name:  "initial status and history replayed",
			query: "?offset=1",
			service: &mockService{openLogStreamFunc: func(_ context.Context, _ string, offset int) (*LogStreamSession, error) {
				assert.Equal(t, 1, offset)
				return &LogStreamSession{
					InitialStatus: StreamStatusEvent{Status: entities.StatusPending},
					History:       history,
					LiveLogs:      liveLogs,
					StatusUpdates: statusUpdates,
					Close:         func() {},
				}, nil
			}},
			wantStatus: http.StatusOK,
			wantBody:   []string{"event:status", "event:log", "id: 0", "history", "live"},
		},
		{
			name:  "not found",
			query: "",
			service: &mockService{openLogStreamFunc: func(context.Context, string, int) (*LogStreamSession, error) {
				return nil, apperrors.NewNotFoundError("deployment not found")
			}},
			wantStatus: http.StatusNotFound,
			wantMsg:    "deployment not found",
		},
		{
			name:       "bad offset",
			query:      "?offset=nope",
			service:    &mockService{},
			wantStatus: http.StatusBadRequest,
			wantMsg:    "offset must be a valid integer",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			router, _ := setupHandler(tc.service)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/deployments/dep_1/logs/stream"+tc.query, nil)

			router.ServeHTTP(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code)
			if rec.Code == http.StatusOK {
				body := rec.Body.String()
				for _, expected := range tc.wantBody {
					assert.Contains(t, body, expected)
				}
				assert.Contains(t, rec.Header().Get("Content-Type"), "text/event-stream")
				return
			}
			assert.Equal(t, tc.wantMsg, decodeAPIResponse(t, rec.Body.Bytes()).Message)
		})
	}
}

func setupHandler(svc Service) (*gin.Engine, *Handler) {
	gin.SetMode(gin.TestMode)

	base := &handler.BaseHandler{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	h := NewDeploymentHandler(base, svc)

	router := gin.New()
	api := router.Group("/api/deployments")
	api.POST("", h.Create)
	api.POST("/upload-url", h.CreateUploadURL)
	api.GET("", h.List)
	api.GET("/:id", h.Get)
	api.DELETE("/:id", h.Delete)
	api.POST("/:id/restart", h.Restart)
	api.GET("/:id/logs", h.GetLogs)
	api.GET("/:id/logs/stream", h.StreamLogs)

	return router, h
}

type apiResponseBody struct {
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func decodeAPIResponse(t *testing.T, body []byte) apiResponseBody {
	t.Helper()

	var resp apiResponseBody
	require.NoError(t, json.Unmarshal(body, &resp))
	return resp
}
