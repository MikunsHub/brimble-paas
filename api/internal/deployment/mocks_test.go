package deployment

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/brimble/paas/entities"
	"github.com/brimble/paas/internal/deployment/builder"
	"github.com/brimble/paas/pkg/broker"
	"github.com/docker/docker/api/types/container"
)

type mockService struct {
	createFunc          func(context.Context, CreateDeploymentRequest) (*entities.Deployment, error)
	createUploadURLFunc func(context.Context, CreateUploadURLRequest) (*CreateUploadURLResponse, error)
	getFunc             func(context.Context, string) (*entities.Deployment, error)
	listFunc            func(context.Context) ([]*entities.Deployment, error)
	teardownFunc        func(context.Context, string) error
	restartFunc         func(context.Context, string) (*entities.Deployment, error)
	getLogsFunc         func(context.Context, string, int) ([]*entities.DeploymentLog, error)
	openLogStreamFunc   func(context.Context, string, int) (*LogStreamSession, error)
}

func (m *mockService) Create(ctx context.Context, req CreateDeploymentRequest) (*entities.Deployment, error) {
	if m.createFunc == nil {
		return nil, nil
	}
	return m.createFunc(ctx, req)
}

func (m *mockService) CreateUploadURL(ctx context.Context, req CreateUploadURLRequest) (*CreateUploadURLResponse, error) {
	if m.createUploadURLFunc == nil {
		return nil, nil
	}
	return m.createUploadURLFunc(ctx, req)
}

func (m *mockService) Get(ctx context.Context, id string) (*entities.Deployment, error) {
	if m.getFunc == nil {
		return nil, nil
	}
	return m.getFunc(ctx, id)
}

func (m *mockService) List(ctx context.Context) ([]*entities.Deployment, error) {
	if m.listFunc == nil {
		return nil, nil
	}
	return m.listFunc(ctx)
}

func (m *mockService) Teardown(ctx context.Context, id string) error {
	if m.teardownFunc == nil {
		return nil
	}
	return m.teardownFunc(ctx, id)
}

func (m *mockService) Restart(ctx context.Context, id string) (*entities.Deployment, error) {
	if m.restartFunc == nil {
		return nil, nil
	}
	return m.restartFunc(ctx, id)
}

func (m *mockService) GetLogs(ctx context.Context, id string, offset int) ([]*entities.DeploymentLog, error) {
	if m.getLogsFunc == nil {
		return nil, nil
	}
	return m.getLogsFunc(ctx, id, offset)
}

func (m *mockService) OpenLogStream(ctx context.Context, id string, offset int) (*LogStreamSession, error) {
	if m.openLogStreamFunc == nil {
		return nil, nil
	}
	return m.openLogStreamFunc(ctx, id, offset)
}

type mockRepo struct {
	mu sync.Mutex

	createFunc    func(context.Context, *entities.Deployment) error
	getByIDFunc   func(context.Context, string) (*entities.Deployment, error)
	listFunc      func(context.Context) ([]*entities.Deployment, error)
	updateFunc    func(context.Context, *entities.Deployment) error
	deleteFunc    func(context.Context, string) error
	insertLogFunc func(context.Context, *entities.DeploymentLog) error
	getLogsFunc   func(context.Context, string, int) ([]*entities.DeploymentLog, error)

	created []*entities.Deployment
	updated []*entities.Deployment
	logs    []*entities.DeploymentLog
}

func (m *mockRepo) Create(ctx context.Context, d *entities.Deployment) error {
	m.mu.Lock()
	m.created = append(m.created, cloneDeployment(d))
	m.mu.Unlock()
	if m.createFunc == nil {
		return nil
	}
	return m.createFunc(ctx, d)
}

func (m *mockRepo) GetByID(ctx context.Context, id string) (*entities.Deployment, error) {
	if m.getByIDFunc == nil {
		return nil, nil
	}
	return m.getByIDFunc(ctx, id)
}

func (m *mockRepo) List(ctx context.Context) ([]*entities.Deployment, error) {
	if m.listFunc == nil {
		return nil, nil
	}
	return m.listFunc(ctx)
}

func (m *mockRepo) Update(ctx context.Context, d *entities.Deployment) error {
	m.mu.Lock()
	m.updated = append(m.updated, cloneDeployment(d))
	m.mu.Unlock()
	if m.updateFunc == nil {
		return nil
	}
	return m.updateFunc(ctx, d)
}

func (m *mockRepo) Delete(ctx context.Context, id string) error {
	if m.deleteFunc == nil {
		return nil
	}
	return m.deleteFunc(ctx, id)
}

func (m *mockRepo) InsertLog(ctx context.Context, l *entities.DeploymentLog) error {
	m.mu.Lock()
	m.logs = append(m.logs, l)
	m.mu.Unlock()
	if m.insertLogFunc == nil {
		return nil
	}
	return m.insertLogFunc(ctx, l)
}

func (m *mockRepo) GetLogs(ctx context.Context, deploymentID string, offset int) ([]*entities.DeploymentLog, error) {
	if m.getLogsFunc == nil {
		return nil, nil
	}
	return m.getLogsFunc(ctx, deploymentID, offset)
}

type mockS3 struct {
	createDeploymentUploadURLFunc func(context.Context, string, string, time.Duration) (string, string, error)
	uploadFunc                    func(context.Context, string, io.Reader, string) error
	downloadFunc                  func(context.Context, string) (io.ReadCloser, error)
	existsFunc                    func(context.Context, string) (bool, error)
}

func (m *mockS3) CreateDeploymentUploadURL(ctx context.Context, fileName, contentType string, expires time.Duration) (string, string, error) {
	if m.createDeploymentUploadURLFunc == nil {
		return "", "", nil
	}
	return m.createDeploymentUploadURLFunc(ctx, fileName, contentType, expires)
}

func (m *mockS3) Upload(ctx context.Context, key string, body io.Reader, contentType string) error {
	if m.uploadFunc == nil {
		return nil
	}
	return m.uploadFunc(ctx, key, body, contentType)
}

func (m *mockS3) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	if m.downloadFunc == nil {
		return io.NopCloser(&emptyReader{}), nil
	}
	return m.downloadFunc(ctx, key)
}

func (m *mockS3) Exists(ctx context.Context, key string) (bool, error) {
	if m.existsFunc == nil {
		return false, nil
	}
	return m.existsFunc(ctx, key)
}

type mockBroker struct {
	publishFunc   func(string, broker.LogLine) error
	subscribeFunc func(string) (<-chan broker.LogLine, func(), error)
}

func (m *mockBroker) Publish(deploymentID string, line broker.LogLine) error {
	if m.publishFunc == nil {
		return nil
	}
	return m.publishFunc(deploymentID, line)
}

func (m *mockBroker) Subscribe(deploymentID string) (<-chan broker.LogLine, func(), error) {
	if m.subscribeFunc == nil {
		ch := make(chan broker.LogLine)
		return ch, func() { close(ch) }, nil
	}
	return m.subscribeFunc(deploymentID)
}

type mockBuilder struct {
	cloneFunc func(context.Context, string, string, string) error
	buildFunc func(context.Context, string, string, string) (*builder.BuildInfo, error)
}

func (m *mockBuilder) Clone(ctx context.Context, gitURL, destDir, deploymentID string) error {
	if m.cloneFunc == nil {
		return nil
	}
	return m.cloneFunc(ctx, gitURL, destDir, deploymentID)
}

func (m *mockBuilder) Build(ctx context.Context, sourceDir, imageTag, deploymentID string) (*builder.BuildInfo, error) {
	if m.buildFunc == nil {
		return &builder.BuildInfo{}, nil
	}
	return m.buildFunc(ctx, sourceDir, imageTag, deploymentID)
}

type mockDocker struct {
	runContainerFunc      func(context.Context, string, string) (string, string, error)
	stopContainerFunc     func(context.Context, string) error
	inspectContainerFunc  func(context.Context, string) (*container.State, error)
	waitForHealthyFunc    func(context.Context, string, time.Duration) error
	getContainerLogsFunc  func(context.Context, string) (string, error)
	streamContainerLogsFn func(context.Context, string) (io.ReadCloser, error)
}

func (m *mockDocker) RunContainer(ctx context.Context, imageTag, subdomain string) (string, string, error) {
	if m.runContainerFunc == nil {
		return "", "", nil
	}
	return m.runContainerFunc(ctx, imageTag, subdomain)
}

func (m *mockDocker) StopContainer(ctx context.Context, containerID string) error {
	if m.stopContainerFunc == nil {
		return nil
	}
	return m.stopContainerFunc(ctx, containerID)
}

func (m *mockDocker) InspectContainer(ctx context.Context, containerID string) (*container.State, error) {
	if m.inspectContainerFunc == nil {
		return nil, nil
	}
	return m.inspectContainerFunc(ctx, containerID)
}

func (m *mockDocker) WaitForHealthy(ctx context.Context, containerID string, timeout time.Duration) error {
	if m.waitForHealthyFunc == nil {
		return nil
	}
	return m.waitForHealthyFunc(ctx, containerID, timeout)
}

func (m *mockDocker) GetContainerLogs(ctx context.Context, containerID string) (string, error) {
	if m.getContainerLogsFunc == nil {
		return "", nil
	}
	return m.getContainerLogsFunc(ctx, containerID)
}

func (m *mockDocker) StreamContainerLogs(ctx context.Context, containerID string) (io.ReadCloser, error) {
	if m.streamContainerLogsFn == nil {
		return io.NopCloser(&emptyReader{}), nil
	}
	return m.streamContainerLogsFn(ctx, containerID)
}

type mockRouter struct {
	addRouteFunc    func(context.Context, string, string) error
	removeRouteFunc func(context.Context, string) error
}

func (m *mockRouter) AddRoute(ctx context.Context, subdomain, upstreamAddr string) error {
	if m.addRouteFunc == nil {
		return nil
	}
	return m.addRouteFunc(ctx, subdomain, upstreamAddr)
}

func (m *mockRouter) RemoveRoute(ctx context.Context, subdomain string) error {
	if m.removeRouteFunc == nil {
		return nil
	}
	return m.removeRouteFunc(ctx, subdomain)
}

type emptyReader struct{}

func (r *emptyReader) Read(p []byte) (int, error) {
	return 0, io.EOF
}
