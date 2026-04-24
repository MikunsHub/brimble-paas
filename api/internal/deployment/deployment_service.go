package deployment

import (
	"context"

	"github.com/brimble/paas/config"
	"github.com/brimble/paas/entities"
	"github.com/brimble/paas/internal/naming"
	"github.com/brimble/paas/pkg/broker"
	apperrors "github.com/brimble/paas/pkg/errors"
	"github.com/brimble/paas/pkg/logger"
)

type Service interface {
	Create(ctx context.Context, req CreateDeploymentRequest) (*entities.Deployment, error)
	Get(ctx context.Context, id string) (*entities.Deployment, error)
	List(ctx context.Context) ([]*entities.Deployment, error)
	Teardown(ctx context.Context, id string) error
	GetLogs(ctx context.Context, id string, offset int) ([]*entities.DeploymentLog, error)
}

type deploymentService struct {
	repo   Repository
	broker broker.LogPublisher
	cfg    *config.Config
}

func NewDeploymentService(repo Repository, b broker.LogPublisher, cfg *config.Config) Service {
	return &deploymentService{repo: repo, broker: b, cfg: cfg}
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

	// TODO: stop container (Docker SDK), remove Caddy route, delete S3 source archive

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
// Phases: source acquisition → Railpack build → Docker run → health check → Caddy route.
func (s *deploymentService) runPipeline(d *entities.Deployment) {
	ctx := context.Background()
	logger.Info("pipeline started", "id", d.ID, "subdomain", d.Subdomain)

	d.Status = entities.StatusBuilding
	if err := s.repo.Update(ctx, d); err != nil {
		logger.Error(err, "pipeline: failed to update status", "id", d.ID)
	}
}
