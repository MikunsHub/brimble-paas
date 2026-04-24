package deployment

import (
	"time"

	"github.com/brimble/paas/entities"
)

type CreateDeploymentRequest struct {
	GitURL *string `json:"git_url"`
}

type DeploymentResponse struct {
	ID        string  `json:"id"`
	Status    string  `json:"status"`
	Subdomain string  `json:"subdomain"`
	LiveURL   *string `json:"live_url,omitempty"`
	ImageTag  *string `json:"image_tag,omitempty"`
	CreatedAt string  `json:"created_at"`
}

type LogResponse struct {
	ID           string `json:"id"`
	DeploymentID string `json:"deployment_id"`
	Timestamp    string `json:"timestamp"`
	Stream       string `json:"stream"`
	Phase        string `json:"phase"`
	Content      string `json:"content"`
}

func toDeploymentResponse(d *entities.Deployment) DeploymentResponse {
	return DeploymentResponse{
		ID:        d.ID,
		Status:    string(d.Status),
		Subdomain: d.Subdomain,
		LiveURL:   d.LiveURL,
		ImageTag:  d.ImageTag,
		CreatedAt: d.CreatedAt.Format(time.RFC3339),
	}
}

func toLogResponse(l *entities.DeploymentLog) LogResponse {
	return LogResponse{
		ID:           l.ID,
		DeploymentID: l.DeploymentID,
		Timestamp:    l.Timestamp.Format(time.RFC3339),
		Stream:       l.Stream,
		Phase:        l.Phase,
		Content:      l.Content,
	}
}
