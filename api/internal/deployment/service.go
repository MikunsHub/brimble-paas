package deployment

import (
	"context"

	"github.com/brimble/paas/entities"
)

type Service interface {
	Create(ctx context.Context, req CreateDeploymentRequest) (*entities.Deployment, error)
	Get(ctx context.Context, id string) (*entities.Deployment, error)
	List(ctx context.Context) ([]*entities.Deployment, error)
	Teardown(ctx context.Context, id string) error
	GetLogs(ctx context.Context, id string, offset int) ([]*entities.DeploymentLog, error)
}
