package deployment

import (
	"context"
	"errors"

	"github.com/brimble/paas/entities"
	"gorm.io/gorm"
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

type postgresRepo struct {
	db *gorm.DB
}

func NewDeploymentRepo(db *gorm.DB) Repository {
	return &postgresRepo{db: db}
}

func (r *postgresRepo) Create(ctx context.Context, d *entities.Deployment) error {
	return r.db.WithContext(ctx).Create(d).Error
}

func (r *postgresRepo) GetByID(ctx context.Context, id string) (*entities.Deployment, error) {
	var d entities.Deployment
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&d).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &d, err
}

func (r *postgresRepo) List(ctx context.Context) ([]*entities.Deployment, error) {
	var deployments []*entities.Deployment
	err := r.db.WithContext(ctx).
		Order("created_at DESC").
		Find(&deployments).Error
	return deployments, err
}

func (r *postgresRepo) Update(ctx context.Context, d *entities.Deployment) error {
	return r.db.WithContext(ctx).Save(d).Error
}

func (r *postgresRepo) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&entities.Deployment{}).Error
}

// ── Deployment logs ───────────────────────────────────────────────────────────

func (r *postgresRepo) InsertLog(ctx context.Context, l *entities.DeploymentLog) error {
	return r.db.WithContext(ctx).Create(l).Error
}

// GetLogs returns logs ordered by timestamp.
// offset skips already-seen rows — used by the SSE handler to replay history
// from a reconnect point without re-sending lines the client already received.
func (r *postgresRepo) GetLogs(ctx context.Context, deploymentID string, offset int) ([]*entities.DeploymentLog, error) {
	var logs []*entities.DeploymentLog
	err := r.db.WithContext(ctx).
		Where("deployment_id = ?", deploymentID).
		Order("timestamp ASC").
		Offset(offset).
		Find(&logs).Error
	return logs, err
}
