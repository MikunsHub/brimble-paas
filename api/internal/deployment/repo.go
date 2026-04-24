package deployment

import (
	"context"

	"github.com/brimble/paas/entities"
)

type Repository interface {
	Create(ctx context.Context, d *entities.Deployment) error
	GetByID(ctx context.Context, id string) (*entities.Deployment, error)
	List(ctx context.Context) ([]*entities.Deployment, error)
	Update(ctx context.Context, d *entities.Deployment) error
	Delete(ctx context.Context, id string) error

	InsertLog(ctx context.Context, l *entities.DeploymentLog) error
	GetLogs(ctx context.Context, deploymentID string, offset int) ([]*entities.DeploymentLog, error)
}
