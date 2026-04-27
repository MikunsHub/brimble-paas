package deployment

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/brimble/paas/config"
	"github.com/brimble/paas/entities"
	"github.com/brimble/paas/internal/deployment/builder"
	"github.com/brimble/paas/pkg/broker"
	apperrors "github.com/brimble/paas/pkg/errors"
	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeploymentService_Create(t *testing.T) {
	t.Parallel()

	gitURL := "https://github.com/brimble/app.git"
	filePath := "uploads/app.zip"

	tests := []struct {
		name       string
		req        CreateDeploymentRequest
		repo       *mockRepo
		s3         *mockS3
		builderSvc *mockBuilder
		wantErrMsg string
		verify     func(*testing.T, *entities.Deployment, *mockRepo)
	}{
		{
			name: "git url success",
			req:  CreateDeploymentRequest{GitURL: &gitURL},
			repo: &mockRepo{createFunc: func(_ context.Context, d *entities.Deployment) error {
				d.ID = "dep_git"
				return nil
			}},
			s3: &mockS3{},
			builderSvc: &mockBuilder{
				buildFunc: func(context.Context, string, string, string) (*builder.BuildInfo, error) {
					return nil, errors.New("stop pipeline")
				},
			},
			verify: func(t *testing.T, got *entities.Deployment, repo *mockRepo) {
				require.NotNil(t, got)
				assert.Equal(t, entities.StatusPending, got.Status)
				require.NotNil(t, got.GitURL)
				assert.Equal(t, gitURL, *got.GitURL)
				assert.Len(t, repo.created, 1)
			},
		},
		{
			name: "file path success",
			req:  CreateDeploymentRequest{FilePath: &filePath},
			repo: &mockRepo{createFunc: func(_ context.Context, d *entities.Deployment) error {
				d.ID = "dep_file"
				return nil
			}},
			s3: &mockS3{existsFunc: func(context.Context, string) (bool, error) { return true, nil }},
			builderSvc: &mockBuilder{
				buildFunc: func(context.Context, string, string, string) (*builder.BuildInfo, error) {
					return nil, errors.New("stop pipeline")
				},
			},
			verify: func(t *testing.T, got *entities.Deployment, repo *mockRepo) {
				require.NotNil(t, got)
				require.NotNil(t, got.S3Key)
				assert.Equal(t, filePath, *got.S3Key)
				assert.Equal(t, entities.StatusPending, got.Status)
				assert.Len(t, repo.created, 1)
			},
		},
		{
			name:       "file path missing on s3",
			req:        CreateDeploymentRequest{FilePath: &filePath},
			repo:       &mockRepo{},
			s3:         &mockS3{existsFunc: func(context.Context, string) (bool, error) { return false, nil }},
			builderSvc: &mockBuilder{},
			wantErrMsg: "file_path does not exist",
		},
		{
			name:       "both provided",
			req:        CreateDeploymentRequest{GitURL: &gitURL, FilePath: &filePath},
			repo:       &mockRepo{},
			s3:         &mockS3{},
			builderSvc: &mockBuilder{},
			wantErrMsg: "provide exactly one of git_url or file_path",
		},
		{
			name:       "neither provided",
			req:        CreateDeploymentRequest{},
			repo:       &mockRepo{},
			s3:         &mockS3{},
			builderSvc: &mockBuilder{},
			wantErrMsg: "provide exactly one of git_url or file_path",
		},
		{
			name: "repo create error",
			req:  CreateDeploymentRequest{GitURL: &gitURL},
			repo: &mockRepo{createFunc: func(context.Context, *entities.Deployment) error { return errors.New("db down") }},
			s3:   &mockS3{},
			builderSvc: &mockBuilder{
				buildFunc: func(context.Context, string, string, string) (*builder.BuildInfo, error) {
					return nil, errors.New("stop pipeline")
				},
			},
			wantErrMsg: "failed to create deployment",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			svc := newTestDeploymentService(tc.repo, &mockBroker{}, tc.builderSvc, &mockDocker{}, &mockRouter{}, tc.s3)
			got, err := svc.Create(context.Background(), tc.req)

			if tc.wantErrMsg != "" {
				require.Error(t, err)
				assert.Equal(t, tc.wantErrMsg, err.Error())
				return
			}

			require.NoError(t, err)
			tc.verify(t, got, tc.repo)
		})
	}
}

func TestDeploymentService_CreateUploadURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		req        CreateUploadURLRequest
		s3         *mockS3
		wantErrMsg string
		verify     func(*testing.T, *CreateUploadURLResponse)
	}{
		{
			name: "success",
			req:  CreateUploadURLRequest{FileName: "app.zip", ContentType: "application/zip"},
			s3: &mockS3{createDeploymentUploadURLFunc: func(_ context.Context, fileName, contentType string, expires time.Duration) (string, string, error) {
				assert.Equal(t, "app.zip", fileName)
				assert.Equal(t, "application/zip", contentType)
				assert.Equal(t, 15*time.Minute, expires)
				return "uploads/app.zip", "https://example.com/upload", nil
			}},
			verify: func(t *testing.T, resp *CreateUploadURLResponse) {
				require.NotNil(t, resp)
				assert.Equal(t, "PUT", resp.Method)
			},
		},
		{
			name:       "empty filename",
			req:        CreateUploadURLRequest{FileName: "   "},
			s3:         &mockS3{},
			wantErrMsg: "file_name is required",
		},
		{
			name: "s3 error",
			req:  CreateUploadURLRequest{FileName: "app.zip"},
			s3: &mockS3{createDeploymentUploadURLFunc: func(context.Context, string, string, time.Duration) (string, string, error) {
				return "", "", errors.New("boom")
			}},
			wantErrMsg: "failed to create upload url",
		},
		{
			name: "zip error path",
			req:  CreateUploadURLRequest{FileName: "app.bad"},
			s3: &mockS3{createDeploymentUploadURLFunc: func(context.Context, string, string, time.Duration) (string, string, error) {
				return "", "", errors.New("must end with .zip")
			}},
			wantErrMsg: "file must be a .zip archive",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			svc := newTestDeploymentService(&mockRepo{}, &mockBroker{}, &mockBuilder{}, &mockDocker{}, &mockRouter{}, tc.s3)
			got, err := svc.CreateUploadURL(context.Background(), tc.req)

			if tc.wantErrMsg != "" {
				require.Error(t, err)
				assert.Equal(t, tc.wantErrMsg, err.Error())
				return
			}

			require.NoError(t, err)
			tc.verify(t, got)
		})
	}
}

func TestDeploymentService_Get(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		repo       *mockRepo
		wantErrMsg string
	}{
		{
			name: "found",
			repo: &mockRepo{getByIDFunc: func(context.Context, string) (*entities.Deployment, error) {
				return &entities.Deployment{ID: "dep_1"}, nil
			}},
		},
		{
			name:       "not found",
			repo:       &mockRepo{getByIDFunc: func(context.Context, string) (*entities.Deployment, error) { return nil, nil }},
			wantErrMsg: "deployment not found",
		},
		{
			name:       "repo error",
			repo:       &mockRepo{getByIDFunc: func(context.Context, string) (*entities.Deployment, error) { return nil, errors.New("db down") }},
			wantErrMsg: "failed to fetch deployment",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newTestDeploymentService(tc.repo, &mockBroker{}, &mockBuilder{}, &mockDocker{}, &mockRouter{}, &mockS3{})
			got, err := svc.Get(context.Background(), "dep_1")
			if tc.wantErrMsg != "" {
				require.Error(t, err)
				assert.Equal(t, tc.wantErrMsg, err.Error())
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "dep_1", got.ID)
		})
	}
}

func TestDeploymentService_List(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		repo       *mockRepo
		wantErrMsg string
		wantLen    int
	}{
		{
			name: "success",
			repo: &mockRepo{listFunc: func(context.Context) ([]*entities.Deployment, error) {
				return []*entities.Deployment{{ID: "1"}, {ID: "2"}}, nil
			}},
			wantLen: 2,
		},
		{
			name:    "empty",
			repo:    &mockRepo{listFunc: func(context.Context) ([]*entities.Deployment, error) { return []*entities.Deployment{}, nil }},
			wantLen: 0,
		},
		{
			name:       "repo error",
			repo:       &mockRepo{listFunc: func(context.Context) ([]*entities.Deployment, error) { return nil, errors.New("db down") }},
			wantErrMsg: "failed to list deployments",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newTestDeploymentService(tc.repo, &mockBroker{}, &mockBuilder{}, &mockDocker{}, &mockRouter{}, &mockS3{})
			got, err := svc.List(context.Background())
			if tc.wantErrMsg != "" {
				require.Error(t, err)
				assert.Equal(t, tc.wantErrMsg, err.Error())
				return
			}
			require.NoError(t, err)
			assert.Len(t, got, tc.wantLen)
		})
	}
}

func TestDeploymentService_Teardown(t *testing.T) {
	t.Parallel()

	containerID := "1234567890abcdef"
	deployment := &entities.Deployment{ID: "dep_1", Subdomain: "demo", Status: entities.StatusRunning, ContainerID: &containerID}

	tests := []struct {
		name           string
		repo           *mockRepo
		dockerSvc      *mockDocker
		router         *mockRouter
		wantErrMsg     string
		wantUpdateCall bool
	}{
		{
			name: "success",
			repo: &mockRepo{
				getByIDFunc: func(context.Context, string) (*entities.Deployment, error) { return cloneDeployment(deployment), nil },
			},
			dockerSvc:      &mockDocker{},
			router:         &mockRouter{},
			wantUpdateCall: true,
		},
		{
			name:       "not found",
			repo:       &mockRepo{getByIDFunc: func(context.Context, string) (*entities.Deployment, error) { return nil, nil }},
			dockerSvc:  &mockDocker{},
			router:     &mockRouter{},
			wantErrMsg: "deployment not found",
		},
		{
			name: "docker stop error continues",
			repo: &mockRepo{
				getByIDFunc: func(context.Context, string) (*entities.Deployment, error) { return cloneDeployment(deployment), nil },
			},
			dockerSvc:      &mockDocker{stopContainerFunc: func(context.Context, string) error { return errors.New("stop failed") }},
			router:         &mockRouter{},
			wantUpdateCall: true,
		},
		{
			name: "caddy remove error continues",
			repo: &mockRepo{
				getByIDFunc: func(context.Context, string) (*entities.Deployment, error) { return cloneDeployment(deployment), nil },
			},
			dockerSvc:      &mockDocker{},
			router:         &mockRouter{removeRouteFunc: func(context.Context, string) error { return errors.New("remove failed") }},
			wantUpdateCall: true,
		},
		{
			name: "repo update error",
			repo: &mockRepo{
				getByIDFunc: func(context.Context, string) (*entities.Deployment, error) { return cloneDeployment(deployment), nil },
				updateFunc:  func(context.Context, *entities.Deployment) error { return errors.New("update failed") },
			},
			dockerSvc:  &mockDocker{},
			router:     &mockRouter{},
			wantErrMsg: "failed to update deployment status",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newTestDeploymentService(tc.repo, &mockBroker{}, &mockBuilder{}, tc.dockerSvc, tc.router, &mockS3{})
			err := svc.Teardown(context.Background(), "dep_1")
			if tc.wantErrMsg != "" {
				require.Error(t, err)
				assert.Equal(t, tc.wantErrMsg, err.Error())
				return
			}
			require.NoError(t, err)
			if tc.wantUpdateCall {
				require.NotEmpty(t, tc.repo.updated)
				assert.Equal(t, entities.StatusStopped, tc.repo.updated[len(tc.repo.updated)-1].Status)
			}
		})
	}
}

func TestDeploymentService_Restart(t *testing.T) {
	t.Parallel()

	imageTag := "demo:latest"
	gitURL := "https://github.com/brimble/app.git"

	tests := []struct {
		name       string
		deployment *entities.Deployment
		repo       *mockRepo
		builderSvc *mockBuilder
		dockerSvc  *mockDocker
		wantErrMsg string
	}{
		{
			name:       "success from existing image",
			deployment: &entities.Deployment{ID: "dep_1", Subdomain: "demo", Status: entities.StatusStopped, ImageTag: &imageTag},
			repo:       &mockRepo{},
			builderSvc: &mockBuilder{},
			dockerSvc: &mockDocker{runContainerFunc: func(context.Context, string, string) (string, string, error) {
				return "", "", errors.New("stop pipeline")
			}},
		},
		{
			name:       "success from pipeline",
			deployment: &entities.Deployment{ID: "dep_1", Subdomain: "demo", Status: entities.StatusFailed, GitURL: &gitURL},
			repo:       &mockRepo{},
			builderSvc: &mockBuilder{buildFunc: func(context.Context, string, string, string) (*builder.BuildInfo, error) {
				return nil, errors.New("stop pipeline")
			}},
			dockerSvc: &mockDocker{},
		},
		{
			name:       "not restartable state",
			deployment: &entities.Deployment{ID: "dep_1", Subdomain: "demo", Status: entities.StatusRunning, GitURL: &gitURL},
			repo:       &mockRepo{},
			builderSvc: &mockBuilder{},
			dockerSvc:  &mockDocker{},
			wantErrMsg: "deployment is not in a restartable state",
		},
		{
			name:       "no source available",
			deployment: &entities.Deployment{ID: "dep_1", Subdomain: "demo", Status: entities.StatusStopped},
			repo:       &mockRepo{},
			builderSvc: &mockBuilder{},
			dockerSvc:  &mockDocker{},
			wantErrMsg: "no source available to restart from",
		},
		{
			name:       "not found",
			deployment: nil,
			repo:       &mockRepo{},
			builderSvc: &mockBuilder{},
			dockerSvc:  &mockDocker{},
			wantErrMsg: "deployment not found",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tc.repo.getByIDFunc = func(context.Context, string) (*entities.Deployment, error) {
				return cloneDeployment(tc.deployment), nil
			}

			svc := newTestDeploymentService(tc.repo, &mockBroker{}, tc.builderSvc, tc.dockerSvc, &mockRouter{}, &mockS3{})
			got, err := svc.Restart(context.Background(), "dep_1")
			if tc.wantErrMsg != "" {
				require.Error(t, err)
				assert.Equal(t, tc.wantErrMsg, err.Error())
				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, entities.StatusPending, got.Status)
			require.NotEmpty(t, tc.repo.updated)
			first := tc.repo.updated[0]
			assert.Equal(t, entities.StatusPending, first.Status)
			assert.Nil(t, first.ContainerID)
			assert.Nil(t, first.ContainerAddr)
			assert.Nil(t, first.ErrorMessage)
		})
	}
}

func TestDeploymentService_GetLogs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		repo       *mockRepo
		wantErrMsg string
		wantLen    int
	}{
		{
			name: "success",
			repo: &mockRepo{getLogsFunc: func(context.Context, string, int) ([]*entities.DeploymentLog, error) {
				return []*entities.DeploymentLog{{ID: "log_1"}}, nil
			}},
			wantLen: 1,
		},
		{
			name: "repo error",
			repo: &mockRepo{getLogsFunc: func(context.Context, string, int) ([]*entities.DeploymentLog, error) {
				return nil, errors.New("db down")
			}},
			wantErrMsg: "failed to fetch logs",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newTestDeploymentService(tc.repo, &mockBroker{}, &mockBuilder{}, &mockDocker{}, &mockRouter{}, &mockS3{})
			got, err := svc.GetLogs(context.Background(), "dep_1", 2)
			if tc.wantErrMsg != "" {
				require.Error(t, err)
				assert.Equal(t, tc.wantErrMsg, err.Error())
				return
			}
			require.NoError(t, err)
			assert.Len(t, got, tc.wantLen)
		})
	}
}

func TestDeploymentService_OpenLogStream(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	deployment := &entities.Deployment{ID: "dep_1", Status: entities.StatusRunning}

	t.Run("success with history and catchup", func(t *testing.T) {
		t.Parallel()

		var getLogsCalls atomic.Int32
		repo := &mockRepo{
			getByIDFunc: func(context.Context, string) (*entities.Deployment, error) {
				return cloneDeployment(deployment), nil
			},
			getLogsFunc: func(context.Context, string, int) ([]*entities.DeploymentLog, error) {
				switch getLogsCalls.Add(1) {
				case 1:
					return []*entities.DeploymentLog{{ID: "h1", DeploymentID: "dep_1", Timestamp: now, Stream: "stdout", Phase: "build", Content: "history"}}, nil
				case 2:
					return []*entities.DeploymentLog{{ID: "c1", DeploymentID: "dep_1", Timestamp: now.Add(time.Second), Stream: "stdout", Phase: "build", Content: "catchup"}}, nil
				default:
					return []*entities.DeploymentLog{}, nil
				}
			},
		}

		liveSource := make(chan broker.LogLine, 2)
		liveSource <- broker.LogLine{ID: "c1", DeploymentID: "dep_1", Timestamp: now.Add(2 * time.Second).Format(time.RFC3339), Stream: "stdout", Phase: "build", Content: "duplicate"}
		liveSource <- broker.LogLine{ID: "l1", DeploymentID: "dep_1", Timestamp: now.Add(3 * time.Second).Format(time.RFC3339), Stream: "stdout", Phase: "runtime", Content: "live"}
		close(liveSource)

		var unsubscribed atomic.Bool
		b := &mockBroker{
			subscribeFunc: func(string) (<-chan broker.LogLine, func(), error) {
				return liveSource, func() { unsubscribed.Store(true) }, nil
			},
		}

		svc := newTestDeploymentService(repo, b, &mockBuilder{}, &mockDocker{}, &mockRouter{}, &mockS3{})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		session, err := svc.OpenLogStream(ctx, "dep_1", 0)
		require.NoError(t, err)
		require.NotNil(t, session)
		assert.Equal(t, entities.StatusRunning, session.InitialStatus.Status)
		require.Len(t, session.History, 2)
		assert.Equal(t, "history", session.History[0].Content)
		assert.Equal(t, "catchup", session.History[1].Content)

		select {
		case event, ok := <-session.LiveLogs:
			require.True(t, ok)
			assert.Equal(t, "live", event.Content)
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for live log")
		}

		session.Close()
		cancel()
		require.Eventually(t, unsubscribed.Load, time.Second, 10*time.Millisecond)
	})

	t.Run("negative offset", func(t *testing.T) {
		t.Parallel()
		svc := newTestDeploymentService(&mockRepo{}, &mockBroker{}, &mockBuilder{}, &mockDocker{}, &mockRouter{}, &mockS3{})
		_, err := svc.OpenLogStream(context.Background(), "dep_1", -1)
		require.Error(t, err)
		assert.Equal(t, "offset must be greater than or equal to 0", err.Error())
	})

	t.Run("deployment not found", func(t *testing.T) {
		t.Parallel()
		repo := &mockRepo{getByIDFunc: func(context.Context, string) (*entities.Deployment, error) { return nil, nil }}
		svc := newTestDeploymentService(repo, &mockBroker{}, &mockBuilder{}, &mockDocker{}, &mockRouter{}, &mockS3{})
		_, err := svc.OpenLogStream(context.Background(), "dep_1", 0)
		require.Error(t, err)
		assert.Equal(t, "deployment not found", err.Error())
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()
		repo := &mockRepo{getByIDFunc: func(context.Context, string) (*entities.Deployment, error) { return nil, errors.New("db down") }}
		svc := newTestDeploymentService(repo, &mockBroker{}, &mockBuilder{}, &mockDocker{}, &mockRouter{}, &mockS3{})
		_, err := svc.OpenLogStream(context.Background(), "dep_1", 0)
		require.Error(t, err)
		assert.Equal(t, "failed to fetch deployment", err.Error())
	})
}

func TestDeploymentHelpers(t *testing.T) {
	t.Parallel()

	t.Run("is terminal status", func(t *testing.T) {
		t.Parallel()
		assert.True(t, isTerminalStatus(entities.StatusFailed))
		assert.True(t, isTerminalStatus(entities.StatusStopped))
		assert.False(t, isTerminalStatus(entities.StatusRunning))
	})

	t.Run("string ptr value", func(t *testing.T) {
		t.Parallel()
		value := "hello"
		assert.Equal(t, "", stringPtrValue(nil))
		assert.Equal(t, "hello", stringPtrValue(&value))
	})

	t.Run("deployment to status event", func(t *testing.T) {
		t.Parallel()
		errMsg := "boom"
		liveURL := "http://demo.example.com"
		event := deploymentToStatusEvent(&entities.Deployment{Status: entities.StatusRunning, LiveURL: &liveURL, ErrorMessage: &errMsg})
		assert.Equal(t, entities.StatusRunning, event.Status)
		assert.Equal(t, liveURL, event.LiveURL)
		assert.Equal(t, errMsg, event.ErrorMessage)
	})

	t.Run("broker log to stream event", func(t *testing.T) {
		t.Parallel()
		event := brokerLogToStreamEvent(broker.LogLine{ID: "log_1", DeploymentID: "dep_1", Timestamp: "2026-04-27T12:00:00Z", Stream: "stderr", Phase: "build", Content: "boom"}, 3)
		assert.Equal(t, 3, event.Index)
		assert.Equal(t, "boom", event.Content)
	})

	t.Run("deployment log to stream event", func(t *testing.T) {
		t.Parallel()
		event := deploymentLogToStreamEvent(&entities.DeploymentLog{ID: "log_1", DeploymentID: "dep_1", Timestamp: time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC), Stream: "stdout", Phase: "build", Content: "done"}, 4)
		assert.Equal(t, 4, event.Index)
		assert.Equal(t, "done", event.Content)
		assert.Equal(t, "2026-04-27T12:00:00Z", event.Timestamp)
	})
}

func strPtr(s string) *string { return &s }

func newTestDeploymentService(repo Repository, b broker.LogPublisher, builderSvc Builder, dockerSvc DockerManager, router Router, s3 s3API) *deploymentService {
	return &deploymentService{
		repo:       repo,
		broker:     b,
		builderSvc: builderSvc,
		dockerSvc:  dockerSvc,
		caddySvc:   router,
		s3:         s3,
		cfg:        &config.Config{Domain: "example.com"},
	}
}

func TestDeploymentService_AppErrorsStayTyped(t *testing.T) {
	t.Parallel()

	err := apperrors.NewBadRequestError("bad")
	assert.Equal(t, "bad", err.Error())
}

func TestShouldSkipDeploymentLog(t *testing.T) {
	t.Parallel()

	assert.False(t, shouldSkipDeploymentLog("build", "normal"))
	assert.True(t, shouldSkipDeploymentLog("runtime", `{"logger":"http.log.access"}`))
	assert.True(t, shouldSkipDeploymentLog("runtime", `{"logger":"http.log.error","request":{"uri":"/favicon.ico"},"status":404}`))
	assert.False(t, shouldSkipDeploymentLog("runtime", "application output"))
}

func TestRuntimeLogWriter_Write(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{}
	var published broker.LogLine
	writer := &runtimeLogWriter{
		deploymentID: "dep_1",
		phase:        "runtime",
		stream:       "stdout",
		broker: &mockBroker{publishFunc: func(_ string, line broker.LogLine) error {
			published = line
			return nil
		}},
		repo: repo,
		ctx:  context.Background(),
	}

	n, err := writer.Write([]byte("hello world"))
	require.NoError(t, err)
	assert.Equal(t, len("hello world"), n)
	require.Len(t, repo.logs, 1)
	assert.Equal(t, "hello world", strings.TrimSpace(published.Content))
}

func TestDownloadObjectToFile(t *testing.T) {
	t.Parallel()

	svc := newTestDeploymentService(&mockRepo{}, &mockBroker{}, &mockBuilder{}, &mockDocker{}, &mockRouter{}, &mockS3{
		downloadFunc: func(context.Context, string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("archive")), nil
		},
	})

	path := t.TempDir() + "/archive.txt"
	require.NoError(t, svc.downloadObjectToFile(context.Background(), "key", path))
}

func TestDownloadObjectToFile_CreateError(t *testing.T) {
	t.Parallel()

	svc := newTestDeploymentService(&mockRepo{}, &mockBroker{}, &mockBuilder{}, &mockDocker{}, &mockRouter{}, &mockS3{
		downloadFunc: func(context.Context, string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("data")), nil
		},
	})

	err := svc.downloadObjectToFile(context.Background(), "key", "/nonexistent/dir/file.txt")
	require.Error(t, err)
}

func TestNewDeploymentService(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{}
	b := &mockBroker{}
	builderSvc := &mockBuilder{}
	dockerSvc := &mockDocker{}
	router := &mockRouter{}
	s3 := &mockS3{}
	cfg := &config.Config{Domain: "test.com"}

	svc := NewDeploymentService(repo, b, builderSvc, dockerSvc, router, s3, cfg)
	require.NotNil(t, svc)

	ds, ok := svc.(*deploymentService)
	require.True(t, ok)
	assert.Equal(t, repo, ds.repo)
	assert.Equal(t, b, ds.broker)
	assert.Equal(t, builderSvc, ds.builderSvc)
	assert.Equal(t, dockerSvc, ds.dockerSvc)
	assert.Equal(t, router, ds.caddySvc)
	assert.Equal(t, s3, ds.s3)
	assert.Equal(t, cfg, ds.cfg)
}

func TestRunPipeline_BuildError(t *testing.T) {
	t.Parallel()

	gitURL := "https://github.com/foo/bar.git"
	repo := &mockRepo{updateFunc: func(context.Context, *entities.Deployment) error { return nil }}
	builderSvc := &mockBuilder{
		cloneFunc: func(_ context.Context, _ string, destDir, _ string) error {
			return os.WriteFile(filepath.Join(destDir, "main.go"), []byte("package main"), 0o644)
		},
		buildFunc: func(context.Context, string, string, string) (*builder.BuildInfo, error) {
			return nil, errors.New("build failed")
		},
	}

	svc := newTestDeploymentService(repo, &mockBroker{}, builderSvc, &mockDocker{}, &mockRouter{}, &mockS3{
		uploadFunc: func(context.Context, string, io.Reader, string) error { return nil },
	})
	d := &entities.Deployment{ID: "dep_pipe_docker", Subdomain: "demo", Status: entities.StatusPending, GitURL: &gitURL}

	svc.runPipeline(d)

	require.NotEmpty(t, repo.updated)
	last := repo.updated[len(repo.updated)-1]
	assert.Equal(t, entities.StatusFailed, last.Status)
	require.NotNil(t, last.ErrorMessage)
	assert.Contains(t, *last.ErrorMessage, "build failed")
}

func TestRunPipeline_DockerRunError(t *testing.T) {
	t.Parallel()

	gitURL := "https://github.com/foo/bar.git"
	repo := &mockRepo{updateFunc: func(context.Context, *entities.Deployment) error { return nil }}
	builderSvc := &mockBuilder{
		cloneFunc: func(_ context.Context, _ string, destDir, _ string) error {
			return os.WriteFile(filepath.Join(destDir, "main.go"), []byte("package main"), 0o644)
		},
		buildFunc: func(context.Context, string, string, string) (*builder.BuildInfo, error) {
			return &builder.BuildInfo{}, nil
		},
	}
	dockerSvc := &mockDocker{
		runContainerFunc: func(context.Context, string, string) (string, string, error) {
			return "", "", errors.New("docker run failed")
		},
	}

	svc := newTestDeploymentService(repo, &mockBroker{}, builderSvc, dockerSvc, &mockRouter{}, &mockS3{
		uploadFunc: func(context.Context, string, io.Reader, string) error { return nil },
	})
	d := &entities.Deployment{ID: "dep_pipe_health", Subdomain: "demo", Status: entities.StatusPending, GitURL: &gitURL}

	svc.runPipeline(d)

	require.NotEmpty(t, repo.updated)
	last := repo.updated[len(repo.updated)-1]
	assert.Equal(t, entities.StatusFailed, last.Status)
	require.NotNil(t, last.ErrorMessage)
	assert.Contains(t, *last.ErrorMessage, "docker run failed")
}

func TestRunPipeline_HealthCheckError(t *testing.T) {
	t.Parallel()

	gitURL := "https://github.com/foo/bar.git"
	repo := &mockRepo{updateFunc: func(context.Context, *entities.Deployment) error { return nil }}
	builderSvc := &mockBuilder{
		cloneFunc: func(_ context.Context, _ string, destDir, _ string) error {
			return os.WriteFile(filepath.Join(destDir, "main.go"), []byte("package main"), 0o644)
		},
		buildFunc: func(context.Context, string, string, string) (*builder.BuildInfo, error) {
			return &builder.BuildInfo{}, nil
		},
	}
	dockerSvc := &mockDocker{
		runContainerFunc: func(context.Context, string, string) (string, string, error) {
			return "container123", "10.0.0.1:8000", nil
		},
		waitForHealthyFunc: func(context.Context, string, time.Duration) error {
			return errors.New("unhealthy")
		},
		stopContainerFunc: func(context.Context, string) error { return nil },
		getContainerLogsFunc: func(context.Context, string) (string, error) {
			return "crash logs", nil
		},
	}

	svc := newTestDeploymentService(repo, &mockBroker{}, builderSvc, dockerSvc, &mockRouter{}, &mockS3{
		uploadFunc: func(context.Context, string, io.Reader, string) error { return nil },
	})
	d := &entities.Deployment{ID: "dep_pipe_caddy", Subdomain: "demo", Status: entities.StatusPending, GitURL: &gitURL}

	svc.runPipeline(d)

	require.NotEmpty(t, repo.updated)
	last := repo.updated[len(repo.updated)-1]
	assert.Equal(t, entities.StatusFailed, last.Status)
	require.NotNil(t, last.ErrorMessage)
	assert.Contains(t, *last.ErrorMessage, "health check failed")
}

func TestRunPipeline_CaddyError(t *testing.T) {
	t.Parallel()

	gitURL := "https://github.com/foo/bar.git"
	repo := &mockRepo{updateFunc: func(context.Context, *entities.Deployment) error { return nil }}
	builderSvc := &mockBuilder{
		cloneFunc: func(_ context.Context, _ string, destDir, _ string) error {
			return os.WriteFile(filepath.Join(destDir, "main.go"), []byte("package main"), 0o644)
		},
		buildFunc: func(context.Context, string, string, string) (*builder.BuildInfo, error) {
			return &builder.BuildInfo{}, nil
		},
	}
	dockerSvc := &mockDocker{
		runContainerFunc: func(context.Context, string, string) (string, string, error) {
			return "container123", "10.0.0.1:8000", nil
		},
		waitForHealthyFunc: func(context.Context, string, time.Duration) error { return nil },
		stopContainerFunc:  func(context.Context, string) error { return nil },
	}
	router := &mockRouter{
		addRouteFunc: func(context.Context, string, string) error {
			return errors.New("caddy failed")
		},
	}

	svc := newTestDeploymentService(repo, &mockBroker{}, builderSvc, dockerSvc, router, &mockS3{
		uploadFunc: func(context.Context, string, io.Reader, string) error { return nil },
	})
	d := &entities.Deployment{ID: "dep_pipe_success", Subdomain: "demo", Status: entities.StatusPending, GitURL: &gitURL}

	svc.runPipeline(d)

	require.NotEmpty(t, repo.updated)
	last := repo.updated[len(repo.updated)-1]
	assert.Equal(t, entities.StatusFailed, last.Status)
	require.NotNil(t, last.ErrorMessage)
	assert.Contains(t, *last.ErrorMessage, "caddy route")
}

func TestRunPipeline_Success(t *testing.T) {
	t.Parallel()

	gitURL := "https://github.com/foo/bar.git"
	repo := &mockRepo{updateFunc: func(context.Context, *entities.Deployment) error { return nil }}
	builderSvc := &mockBuilder{
		cloneFunc: func(_ context.Context, _ string, destDir, _ string) error {
			return os.WriteFile(filepath.Join(destDir, "main.go"), []byte("package main"), 0o644)
		},
		buildFunc: func(context.Context, string, string, string) (*builder.BuildInfo, error) {
			return &builder.BuildInfo{DetectedLang: "go", StartCmd: "./app"}, nil
		},
	}
	dockerSvc := &mockDocker{
		runContainerFunc: func(context.Context, string, string) (string, string, error) {
			return "container123", "10.0.0.1:8000", nil
		},
		waitForHealthyFunc: func(context.Context, string, time.Duration) error { return nil },
		streamContainerLogsFn: func(context.Context, string) (io.ReadCloser, error) {
			return io.NopCloser(&emptyReader{}), nil
		},
	}
	router := &mockRouter{addRouteFunc: func(context.Context, string, string) error { return nil }}

	svc := newTestDeploymentService(repo, &mockBroker{}, builderSvc, dockerSvc, router, &mockS3{
		uploadFunc: func(context.Context, string, io.Reader, string) error { return nil },
	})
	d := &entities.Deployment{ID: "dep_pipe", Subdomain: "demo", Status: entities.StatusPending, GitURL: &gitURL}

	svc.runPipeline(d)

	require.NotEmpty(t, repo.updated)
	last := repo.updated[len(repo.updated)-1]
	assert.Equal(t, entities.StatusRunning, last.Status)
	require.NotNil(t, last.LiveURL)
	assert.Equal(t, "http://demo.example.com", *last.LiveURL)
	require.NotNil(t, last.ContainerID)
	assert.Equal(t, "container123", *last.ContainerID)
	require.NotNil(t, last.ImageTag)
	assert.Equal(t, "demo:latest", *last.ImageTag)
	require.NotNil(t, last.DetectedLang)
	assert.Equal(t, "go", *last.DetectedLang)
	require.NotNil(t, last.StartCmd)
	assert.Equal(t, "./app", *last.StartCmd)
}

func TestRunFromImage_DockerError(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{updateFunc: func(context.Context, *entities.Deployment) error { return nil }}
	imageTag := "demo:latest"
	dockerSvc := &mockDocker{
		runContainerFunc: func(context.Context, string, string) (string, string, error) {
			return "", "", errors.New("docker failed")
		},
	}

	svc := newTestDeploymentService(repo, &mockBroker{}, &mockBuilder{}, dockerSvc, &mockRouter{}, &mockS3{})
	d := &entities.Deployment{ID: "dep_img", Subdomain: "demo", Status: entities.StatusStopped, ImageTag: &imageTag}

	svc.runFromImage(d)

	require.NotEmpty(t, repo.updated)
	last := repo.updated[len(repo.updated)-1]
	assert.Equal(t, entities.StatusFailed, last.Status)
}

func TestRunFromImage_HealthCheckError(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{updateFunc: func(context.Context, *entities.Deployment) error { return nil }}
	imageTag := "demo:latest"
	dockerSvc := &mockDocker{
		runContainerFunc: func(context.Context, string, string) (string, string, error) {
			return "container123", "10.0.0.1:8000", nil
		},
		waitForHealthyFunc: func(context.Context, string, time.Duration) error {
			return errors.New("unhealthy")
		},
		stopContainerFunc:    func(context.Context, string) error { return nil },
		getContainerLogsFunc: func(context.Context, string) (string, error) { return "", nil },
	}

	svc := newTestDeploymentService(repo, &mockBroker{}, &mockBuilder{}, dockerSvc, &mockRouter{}, &mockS3{})
	d := &entities.Deployment{ID: "dep_img", Subdomain: "demo", Status: entities.StatusStopped, ImageTag: &imageTag}

	svc.runFromImage(d)

	require.NotEmpty(t, repo.updated)
	last := repo.updated[len(repo.updated)-1]
	assert.Equal(t, entities.StatusFailed, last.Status)
}

func TestRunFromImage_CaddyError(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{updateFunc: func(context.Context, *entities.Deployment) error { return nil }}
	imageTag := "demo:latest"
	dockerSvc := &mockDocker{
		runContainerFunc: func(context.Context, string, string) (string, string, error) {
			return "container123", "10.0.0.1:8000", nil
		},
		waitForHealthyFunc: func(context.Context, string, time.Duration) error { return nil },
		stopContainerFunc:  func(context.Context, string) error { return nil },
	}
	router := &mockRouter{
		addRouteFunc: func(context.Context, string, string) error {
			return errors.New("caddy failed")
		},
	}

	svc := newTestDeploymentService(repo, &mockBroker{}, &mockBuilder{}, dockerSvc, router, &mockS3{})
	d := &entities.Deployment{ID: "dep_img", Subdomain: "demo", Status: entities.StatusStopped, ImageTag: &imageTag}

	svc.runFromImage(d)

	require.NotEmpty(t, repo.updated)
	last := repo.updated[len(repo.updated)-1]
	assert.Equal(t, entities.StatusFailed, last.Status)
}

func TestRunFromImage_Success(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{updateFunc: func(context.Context, *entities.Deployment) error { return nil }}
	imageTag := "demo:latest"
	dockerSvc := &mockDocker{
		runContainerFunc: func(context.Context, string, string) (string, string, error) {
			return "container123", "10.0.0.1:8000", nil
		},
		waitForHealthyFunc: func(context.Context, string, time.Duration) error { return nil },
		streamContainerLogsFn: func(context.Context, string) (io.ReadCloser, error) {
			return io.NopCloser(&emptyReader{}), nil
		},
	}
	router := &mockRouter{addRouteFunc: func(context.Context, string, string) error { return nil }}

	svc := newTestDeploymentService(repo, &mockBroker{}, &mockBuilder{}, dockerSvc, router, &mockS3{})
	d := &entities.Deployment{ID: "dep_img", Subdomain: "demo", Status: entities.StatusStopped, ImageTag: &imageTag}

	svc.runFromImage(d)

	require.NotEmpty(t, repo.updated)
	last := repo.updated[len(repo.updated)-1]
	assert.Equal(t, entities.StatusRunning, last.Status)
	require.NotNil(t, last.LiveURL)
	assert.Equal(t, "http://demo.example.com", *last.LiveURL)
}

func TestAcquireSource_S3Path(t *testing.T) {
	t.Parallel()

	s3Key := "uploads/app.zip"
	downloaded := false
	repo := &mockRepo{updateFunc: func(context.Context, *entities.Deployment) error { return nil }}
	s3 := &mockS3{
		downloadFunc: func(_ context.Context, key string) (io.ReadCloser, error) {
			assert.Equal(t, s3Key, key)
			// Create a minimal valid zip in memory
			var buf bytes.Buffer
			zw := zip.NewWriter(&buf)
			f, _ := zw.Create("readme.txt")
			f.Write([]byte("hello"))
			zw.Close()
			downloaded = true
			return io.NopCloser(bytes.NewReader(buf.Bytes())), nil
		},
	}

	svc := newTestDeploymentService(repo, &mockBroker{}, &mockBuilder{}, &mockDocker{}, &mockRouter{}, s3)
	d := &entities.Deployment{ID: "dep_src", S3Key: &s3Key}
	sourceDir := t.TempDir()

	err := svc.acquireSource(context.Background(), d, sourceDir)
	require.NoError(t, err)
	assert.True(t, downloaded)

	// Verify zip was extracted
	content, err := os.ReadFile(filepath.Join(sourceDir, "readme.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(content))
}

func TestAcquireSource_GitPath(t *testing.T) {
	t.Parallel()

	gitURL := "https://github.com/foo/bar.git"
	cloned := false
	uploaded := false
	repo := &mockRepo{updateFunc: func(context.Context, *entities.Deployment) error { return nil }}
	builderSvc := &mockBuilder{
		cloneFunc: func(_ context.Context, url, destDir, deploymentID string) error {
			assert.Equal(t, gitURL, url)
			cloned = true
			return os.WriteFile(filepath.Join(destDir, "main.go"), []byte("package main"), 0o644)
		},
	}
	s3 := &mockS3{
		uploadFunc: func(_ context.Context, key string, body io.Reader, contentType string) error {
			assert.Contains(t, key, "/source.tar.gz")
			assert.Equal(t, "application/gzip", contentType)
			uploaded = true
			return nil
		},
	}

	svc := newTestDeploymentService(repo, &mockBroker{}, builderSvc, &mockDocker{}, &mockRouter{}, s3)
	d := &entities.Deployment{ID: "dep_git_success", GitURL: &gitURL}
	sourceDir := t.TempDir()

	err := svc.acquireSource(context.Background(), d, sourceDir)
	require.NoError(t, err)
	assert.True(t, cloned)
	assert.True(t, uploaded)
	require.NotNil(t, d.S3Key)
	assert.Contains(t, *d.S3Key, "/source.tar.gz")
}

func TestAcquireSource_GitPath_UploadError(t *testing.T) {
	t.Parallel()

	gitURL := "https://github.com/foo/bar.git"
	builderSvc := &mockBuilder{
		cloneFunc: func(_ context.Context, url, destDir, deploymentID string) error {
			return os.WriteFile(filepath.Join(destDir, "main.go"), []byte("package main"), 0o644)
		},
	}
	s3 := &mockS3{
		uploadFunc: func(context.Context, string, io.Reader, string) error {
			return errors.New("upload failed")
		},
	}

	svc := newTestDeploymentService(&mockRepo{}, &mockBroker{}, builderSvc, &mockDocker{}, &mockRouter{}, s3)
	d := &entities.Deployment{ID: "dep_git", GitURL: &gitURL}
	sourceDir := t.TempDir()

	err := svc.acquireSource(context.Background(), d, sourceDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload source archive")
}

func TestAcquireSource_NoSource(t *testing.T) {
	t.Parallel()

	svc := newTestDeploymentService(&mockRepo{}, &mockBroker{}, &mockBuilder{}, &mockDocker{}, &mockRouter{}, &mockS3{})
	d := &entities.Deployment{ID: "dep_none"}
	sourceDir := t.TempDir()

	err := svc.acquireSource(context.Background(), d, sourceDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git_url is required")
}

func TestUploadFileToS3_FileOpenError(t *testing.T) {
	t.Parallel()

	svc := newTestDeploymentService(&mockRepo{}, &mockBroker{}, &mockBuilder{}, &mockDocker{}, &mockRouter{}, &mockS3{})
	err := svc.uploadFileToS3(context.Background(), "/nonexistent/file.txt", "key", "text/plain")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open file")
}

func TestFailPipeline_UpdateError(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{
		updateFunc: func(context.Context, *entities.Deployment) error {
			return errors.New("update failed")
		},
	}
	svc := newTestDeploymentService(repo, &mockBroker{}, &mockBuilder{}, &mockDocker{}, &mockRouter{}, &mockS3{})
	d := &entities.Deployment{ID: "dep_fail", Status: entities.StatusPending}

	svc.failPipeline(context.Background(), d, errors.New("something broke"))
	assert.Equal(t, entities.StatusFailed, d.Status)
	require.NotNil(t, d.ErrorMessage)
	assert.Equal(t, "something broke", *d.ErrorMessage)
}

func TestPublishLogLines_EmptyAndWhitespace(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{}
	b := &mockBroker{}
	svc := newTestDeploymentService(repo, b, &mockBuilder{}, &mockDocker{}, &mockRouter{}, &mockS3{})

	svc.publishLogLines(context.Background(), "dep_1", "build", "stdout", "line1\n\n   \nline2\n")

	require.Len(t, repo.logs, 2)
	assert.Equal(t, "line1", repo.logs[0].Content)
	assert.Equal(t, "line2", repo.logs[1].Content)
}

func TestPublishLogLines_SkipCaddyLogs(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{}
	b := &mockBroker{}
	svc := newTestDeploymentService(repo, b, &mockBuilder{}, &mockDocker{}, &mockRouter{}, &mockS3{})

	svc.publishLogLines(context.Background(), "dep_1", "runtime", "stdout", `{"logger":"http.log.access"}`)
	assert.Empty(t, repo.logs)

	svc.publishLogLines(context.Background(), "dep_1", "build", "stdout", `{"logger":"http.log.access"}`)
	require.Len(t, repo.logs, 1)
}

func TestExtractZip(t *testing.T) {
	t.Parallel()

	zipPath := filepath.Join(t.TempDir(), "test.zip")
	zw := zip.NewWriter(nil)

	f, err := os.Create(zipPath)
	require.NoError(t, err)
	zw = zip.NewWriter(f)

	w1, err := zw.Create("myapp/readme.txt")
	require.NoError(t, err)
	w1.Write([]byte("hello"))

	w2, err := zw.Create("myapp/src/main.go")
	require.NoError(t, err)
	w2.Write([]byte("package main"))

	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())

	destDir := t.TempDir()
	err = extractZip(zipPath, destDir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(destDir, "readme.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(content))

	content2, err := os.ReadFile(filepath.Join(destDir, "src", "main.go"))
	require.NoError(t, err)
	assert.Equal(t, "package main", string(content2))
}

func TestExtractZip_PathTraversal(t *testing.T) {
	t.Parallel()

	zipPath := filepath.Join(t.TempDir(), "evil.zip")
	f, err := os.Create(zipPath)
	require.NoError(t, err)
	zw := zip.NewWriter(f)

	w1, err := zw.Create("a.txt")
	require.NoError(t, err)
	w1.Write([]byte("alpha"))

	header := &zip.FileHeader{Name: "../../../etc/passwd"}
	w2, err := zw.CreateHeader(header)
	require.NoError(t, err)
	w2.Write([]byte("root"))

	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())

	destDir := t.TempDir()
	err = extractZip(zipPath, destDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "escapes destination")
}

func TestExtractZip_BadZip(t *testing.T) {
	t.Parallel()

	zipPath := filepath.Join(t.TempDir(), "not-a-zip.txt")
	require.NoError(t, os.WriteFile(zipPath, []byte("not a zip"), 0o644))

	err := extractZip(zipPath, t.TempDir())
	require.Error(t, err)
}

func TestDetectZipRootPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		files    []string
		expected string
	}{
		{"single file no dir", []string{"readme.txt"}, ""},
		{"flat files", []string{"a.txt", "b.txt"}, ""},
		{"common prefix", []string{"myapp/a.txt", "myapp/b.txt"}, "myapp/"},
		{"nested common prefix", []string{"myapp/src/a.go", "myapp/src/b.go"}, "myapp/src/"},
		{"mixed dirs and files", []string{"myapp/", "myapp/main.go"}, "myapp/"},
		{"empty", []string{}, ""},
		{"backslash paths", []string{"myapp\\a.txt", "myapp\\b.txt"}, "myapp/"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var files []*zip.File
			for _, name := range tc.files {
				files = append(files, &zip.File{
					FileHeader: zip.FileHeader{Name: name},
				})
			}
			assert.Equal(t, tc.expected, detectZipRootPrefix(files))
		})
	}
}

func TestNormalizeZipPath(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "path/to/file", normalizeZipPath("path\\to\\file"))
	assert.Equal(t, "path/to/file", normalizeZipPath("path/to/file"))
}

func TestCreateTarGz(t *testing.T) {
	t.Parallel()

	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "a.txt"), []byte("alpha"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(sourceDir, "subdir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "subdir", "b.txt"), []byte("beta"), 0o644))

	outPath := filepath.Join(t.TempDir(), "out.tar.gz")
	err := createTarGz(sourceDir, outPath)
	require.NoError(t, err)

	info, err := os.Stat(outPath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}

func TestCreateTarGz_BadSourceDir(t *testing.T) {
	t.Parallel()

	outPath := filepath.Join(t.TempDir(), "out.tar.gz")
	err := createTarGz("/nonexistent/directory", outPath)
	require.Error(t, err)
}

func TestStreamRuntimeLogs(t *testing.T) {
	t.Parallel()

	logData := append(dockerLogFrame(1, "stdout line\n"), dockerLogFrame(2, "stderr line\n")...)
	repo := &mockRepo{}
	repo.getByIDFunc = func(context.Context, string) (*entities.Deployment, error) {
		return &entities.Deployment{ID: "dep_1", Status: entities.StatusRunning}, nil
	}
	repo.updateFunc = func(context.Context, *entities.Deployment) error { return nil }

	var inspectCalled bool
	dockerSvc := &mockDocker{
		streamContainerLogsFn: func(context.Context, string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(logData)), nil
		},
		inspectContainerFunc: func(context.Context, string) (*container.State, error) {
			inspectCalled = true
			return &container.State{Running: false, ExitCode: 1}, nil
		},
	}

	svc := newTestDeploymentService(repo, &mockBroker{}, &mockBuilder{}, dockerSvc, &mockRouter{}, &mockS3{})

	svc.streamRuntimeLogs(context.Background(), "dep_1", "container123")

	assert.True(t, inspectCalled)
	require.NotEmpty(t, repo.logs)
	require.NotEmpty(t, repo.updated)
	assert.Equal(t, entities.StatusFailed, repo.updated[len(repo.updated)-1].Status)
}

func TestStreamRuntimeLogs_StreamError(t *testing.T) {
	t.Parallel()

	dockerSvc := &mockDocker{
		streamContainerLogsFn: func(context.Context, string) (io.ReadCloser, error) {
			return nil, errors.New("stream failed")
		},
	}

	svc := newTestDeploymentService(&mockRepo{}, &mockBroker{}, &mockBuilder{}, dockerSvc, &mockRouter{}, &mockS3{})

	svc.streamRuntimeLogs(context.Background(), "dep_1", "container123")
}

func TestStreamRuntimeLogs_ContainerStillRunning(t *testing.T) {
	t.Parallel()

	logData := dockerLogFrame(1, "stdout line\n")
	repo := &mockRepo{}

	dockerSvc := &mockDocker{
		streamContainerLogsFn: func(context.Context, string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(logData)), nil
		},
		inspectContainerFunc: func(context.Context, string) (*container.State, error) {
			return &container.State{Running: true, ExitCode: 0}, nil
		},
	}

	svc := newTestDeploymentService(repo, &mockBroker{}, &mockBuilder{}, dockerSvc, &mockRouter{}, &mockS3{})
	svc.streamRuntimeLogs(context.Background(), "dep_1", "container123")

	assert.Empty(t, repo.updated)
}

func TestOpenLogStream_TickerUpdates(t *testing.T) {
	t.Parallel()

	deployment := &entities.Deployment{ID: "dep_1", Status: entities.StatusRunning}

	callCount := 0
	repo := &mockRepo{
		getByIDFunc: func(context.Context, string) (*entities.Deployment, error) {
			callCount++
			if callCount == 1 {
				return cloneDeployment(deployment), nil
			}
			failed := cloneDeployment(deployment)
			failed.Status = entities.StatusFailed
			failed.ErrorMessage = strPtr("boom")
			return failed, nil
		},
		getLogsFunc: func(context.Context, string, int) ([]*entities.DeploymentLog, error) {
			return []*entities.DeploymentLog{}, nil
		},
	}

	liveSource := make(chan broker.LogLine)

	b := &mockBroker{
		subscribeFunc: func(string) (<-chan broker.LogLine, func(), error) {
			return liveSource, func() {}, nil
		},
	}

	svc := newTestDeploymentService(repo, b, &mockBuilder{}, &mockDocker{}, &mockRouter{}, &mockS3{})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	session, err := svc.OpenLogStream(ctx, "dep_1", 0)
	require.NoError(t, err)
	require.NotNil(t, session)

	select {
	case status := <-session.StatusUpdates:
		assert.Equal(t, entities.StatusFailed, status.Status)
		assert.Equal(t, "boom", status.ErrorMessage)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ticker status update")
	}

	session.Close()
	close(liveSource)
}

func dockerLogFrame(stream byte, content string) []byte {
	payload := []byte(content)
	header := []byte{stream, 0, 0, 0, 0, 0, 0, byte(len(payload))}
	return append(header, payload...)
}
