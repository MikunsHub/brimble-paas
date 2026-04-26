package deployment

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/brimble/paas/config"
	"github.com/brimble/paas/entities"
	"github.com/brimble/paas/internal/builder"
	"github.com/brimble/paas/internal/caddy"
	"github.com/brimble/paas/internal/docker"
	"github.com/brimble/paas/internal/naming"
	"github.com/brimble/paas/pkg/broker"
	apperrors "github.com/brimble/paas/pkg/errors"
	"github.com/brimble/paas/pkg/logger"
	"github.com/docker/docker/pkg/stdcopy"
)

type Service interface {
	Create(ctx context.Context, req CreateDeploymentRequest) (*entities.Deployment, error)
	Get(ctx context.Context, id string) (*entities.Deployment, error)
	List(ctx context.Context) ([]*entities.Deployment, error)
	Teardown(ctx context.Context, id string) error
	GetLogs(ctx context.Context, id string, offset int) ([]*entities.DeploymentLog, error)
}

type deploymentService struct {
	repo       Repository
	broker     broker.LogPublisher
	builderSvc *builder.BuilderService
	dockerSvc  *docker.DockerService
	caddySvc   *caddy.CaddyService
	cfg        *config.Config
}

func NewDeploymentService(
	repo Repository,
	b broker.LogPublisher,
	builderSvc *builder.BuilderService,
	dockerSvc *docker.DockerService,
	caddySvc *caddy.CaddyService,
	cfg *config.Config,
) Service {
	return &deploymentService{
		repo:       repo,
		broker:     b,
		builderSvc: builderSvc,
		dockerSvc:  dockerSvc,
		caddySvc:   caddySvc,
		cfg:        cfg,
	}
}

func (s *deploymentService) Create(ctx context.Context, req CreateDeploymentRequest) (*entities.Deployment, error) {
	if req.GitURL == nil {
		return nil, apperrors.NewBadRequestError("git_url is required")
	}

	d := &entities.Deployment{
		GitURL:    req.GitURL,
		Subdomain: naming.GenerateSubDomainSlug(),
		Status:    entities.StatusPending,
	}

	if err := s.repo.Create(ctx, d); err != nil {
		return nil, apperrors.NewInternalError("failed to create deployment")
	}

	logger.Info("deployment created", "id", d.ID, "subdomain", d.Subdomain)

	go s.runPipeline(d)

	return d, nil
}

func (s *deploymentService) Get(ctx context.Context, id string) (*entities.Deployment, error) {
	d, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, apperrors.NewInternalError("failed to fetch deployment")
	}
	if d == nil {
		return nil, apperrors.NewNotFoundError("deployment not found")
	}
	return d, nil
}

func (s *deploymentService) List(ctx context.Context) ([]*entities.Deployment, error) {
	deployments, err := s.repo.List(ctx)
	if err != nil {
		return nil, apperrors.NewInternalError("failed to list deployments")
	}
	return deployments, nil
}

func (s *deploymentService) Teardown(ctx context.Context, id string) error {
	d, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return apperrors.NewInternalError("failed to fetch deployment")
	}
	if d == nil {
		return apperrors.NewNotFoundError("deployment not found")
	}

	if d.ContainerID != nil {
		if err := s.dockerSvc.StopContainer(ctx, *d.ContainerID); err != nil {
			logger.Error(err, "teardown: failed to stop container", "id", id, "containerID", *d.ContainerID)
		}
	}

	if err := s.caddySvc.RemoveRoute(ctx, d.Subdomain); err != nil {
		logger.Error(err, "teardown: failed to remove caddy route", "id", id, "subdomain", d.Subdomain)
	}

	d.Status = entities.StatusStopped
	if err := s.repo.Update(ctx, d); err != nil {
		return apperrors.NewInternalError("failed to update deployment status")
	}

	logger.Info("deployment stopped", "id", id)
	return nil
}

func (s *deploymentService) GetLogs(ctx context.Context, id string, offset int) ([]*entities.DeploymentLog, error) {
	logs, err := s.repo.GetLogs(ctx, id, offset)
	if err != nil {
		return nil, apperrors.NewInternalError("failed to fetch logs")
	}
	return logs, nil
}

// runPipeline is launched as a goroutine after a deployment record is created.
// Phases: clone → build → run container → health check → register Caddy route.
func (s *deploymentService) runPipeline(d *entities.Deployment) {
	ctx := context.Background()
	logger.Info("pipeline started", "id", d.ID, "subdomain", d.Subdomain)

	d.Status = entities.StatusBuilding
	if err := s.repo.Update(ctx, d); err != nil {
		logger.Error(err, "pipeline: failed to update status to building", "id", d.ID)
	}

	sourceDir, err := os.MkdirTemp("", "brimble-"+d.ID+"-")
	if err != nil {
		s.failPipeline(ctx, d, fmt.Errorf("failed to create temp dir: %w", err))
		return
	}
	defer os.RemoveAll(sourceDir)

	if err := s.builderSvc.Clone(ctx, *d.GitURL, sourceDir, d.ID); err != nil {
		s.failPipeline(ctx, d, fmt.Errorf("clone: %w", err))
		return
	}

	imageTag := d.Subdomain + ":latest"
	info, err := s.builderSvc.Build(ctx, sourceDir, imageTag, d.ID)
	if err != nil {
		s.failPipeline(ctx, d, fmt.Errorf("build: %w", err))
		return
	}

	d.ImageTag = &imageTag
	if info.DetectedLang != "" {
		d.DetectedLang = &info.DetectedLang
	}
	if info.StartCmd != "" {
		d.StartCmd = &info.StartCmd
	}

	d.Status = entities.StatusDeploying
	if err := s.repo.Update(ctx, d); err != nil {
		logger.Error(err, "pipeline: failed to update status to deploying", "id", d.ID)
	}

	containerID, containerAddr, err := s.dockerSvc.RunContainer(ctx, imageTag, d.Subdomain)
	if err != nil {
		s.failPipeline(ctx, d, fmt.Errorf("run container: %w", err))
		return
	}

	d.ContainerID = &containerID
	d.ContainerAddr = &containerAddr

	if err := s.dockerSvc.WaitForHealthy(ctx, containerID, 10*time.Second); err != nil {
		logs, logErr := s.dockerSvc.GetContainerLogs(ctx, containerID)
		if logErr == nil && logs != "" {
			s.publishLogLines(ctx, d.ID, "runtime", "stderr", logs)
		}
		_ = s.dockerSvc.StopContainer(ctx, containerID)
		s.failPipeline(ctx, d, fmt.Errorf("health check failed: %w", err))
		return
	}

	if err := s.caddySvc.AddRoute(ctx, d.Subdomain, containerAddr); err != nil {
		_ = s.dockerSvc.StopContainer(ctx, containerID)
		s.failPipeline(ctx, d, fmt.Errorf("caddy route: %w", err))
		return
	}

	liveURL := "http://" + d.Subdomain + "." + s.cfg.Domain
	d.LiveURL = &liveURL
	d.Status = entities.StatusRunning

	if err := s.repo.Update(ctx, d); err != nil {
		logger.Error(err, "pipeline: failed to update status to running", "id", d.ID)
		return
	}

	logger.Info("deployment running", "id", d.ID, "url", liveURL, "container", containerID[:12])

	// ── Phase 4: stream runtime logs ──────────────────────────────────────────
	// Start a background goroutine that tails the container's stdout/stderr
	// and publishes them to the log broker + database. If the container
	// crashes later, update its status to failed.

	go s.streamRuntimeLogs(ctx, d.ID, containerID)
}

func (s *deploymentService) failPipeline(ctx context.Context, d *entities.Deployment, err error) {
	logger.Error(err, "pipeline failed", "id", d.ID)
	d.Status = entities.StatusFailed
	msg := err.Error()
	d.ErrorMessage = &msg
	if updateErr := s.repo.Update(ctx, d); updateErr != nil {
		logger.Error(updateErr, "pipeline: failed to set status to failed", "id", d.ID)
	}
}

func (s *deploymentService) publishLogLines(ctx context.Context, deploymentID, phase, stream, content string) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		s.broker.Publish(deploymentID, broker.LogLine{
			Phase:   phase,
			Stream:  stream,
			Content: line,
		})
		if err := s.repo.InsertLog(ctx, &entities.DeploymentLog{
			DeploymentID: deploymentID,
			Stream:       stream,
			Phase:        phase,
			Content:      line,
		}); err != nil {
			logger.Error(err, "failed to persist container log line", "deploymentID", deploymentID)
		}
	}
}

func (s *deploymentService) streamRuntimeLogs(ctx context.Context, deploymentID, containerID string) {
	logs, err := s.dockerSvc.StreamContainerLogs(ctx, containerID)
	if err != nil {
		logger.Error(err, "failed to start runtime log stream", "deploymentID", deploymentID, "containerID", containerID)
		return
	}
	defer logs.Close()

	stdout := &runtimeLogWriter{
		deploymentID: deploymentID,
		phase:        "runtime",
		stream:       "stdout",
		broker:       s.broker,
		repo:         s.repo,
		ctx:          ctx,
	}
	stderr := &runtimeLogWriter{
		deploymentID: deploymentID,
		phase:        "runtime",
		stream:       "stderr",
		broker:       s.broker,
		repo:         s.repo,
		ctx:          ctx,
	}

	_, err = stdcopy.StdCopy(stdout, stderr, logs)
	if err != nil && ctx.Err() == nil {
		logger.Error(err, "runtime log stream ended", "deploymentID", deploymentID, "containerID", containerID)
	}

	// Stream ended — check if the container crashed after startup.
	state, inspectErr := s.dockerSvc.InspectContainer(ctx, containerID)
	if inspectErr == nil && state != nil && !state.Running && state.ExitCode != 0 {
		logger.Info("container crashed after startup", "deploymentID", deploymentID, "exitCode", state.ExitCode)
		d, err := s.repo.GetByID(ctx, deploymentID)
		if err == nil && d != nil && d.Status == entities.StatusRunning {
			d.Status = entities.StatusFailed
			msg := fmt.Sprintf("container crashed with exit code %d", state.ExitCode)
			d.ErrorMessage = &msg
			if updateErr := s.repo.Update(ctx, d); updateErr != nil {
				logger.Error(updateErr, "failed to update deployment status after crash", "deploymentID", deploymentID)
			}
		}
	}
}

type runtimeLogWriter struct {
	deploymentID string
	phase        string
	stream       string
	broker       broker.LogPublisher
	repo         Repository
	ctx          context.Context
}

func (w *runtimeLogWriter) Write(p []byte) (n int, err error) {
	content := strings.TrimSpace(string(p))
	if content == "" {
		return len(p), nil
	}
	w.broker.Publish(w.deploymentID, broker.LogLine{
		Phase:   w.phase,
		Stream:  w.stream,
		Content: content,
	})
	if insertErr := w.repo.InsertLog(w.ctx, &entities.DeploymentLog{
		DeploymentID: w.deploymentID,
		Stream:       w.stream,
		Phase:        w.phase,
		Content:      content,
	}); insertErr != nil {
		logger.Error(insertErr, "failed to persist runtime log", "deploymentID", w.deploymentID)
	}
	return len(p), nil
}
