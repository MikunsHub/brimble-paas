package deployment

import (
	"time"

	"github.com/brimble/paas/entities"
)

type CreateDeploymentRequest struct {
	GitURL   *string `json:"git_url"`
	FilePath *string `json:"file_path"`
}

type CreateUploadURLRequest struct {
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
}

type CreateUploadURLResponse struct {
	FilePath string `json:"file_path"`
	URL      string `json:"url"`
	Method   string `json:"method"`
}

type DeploymentResponse struct {
	ID           string  `json:"id"`
	Status       string  `json:"status"`
	Subdomain    string  `json:"subdomain"`
	GitURL       *string `json:"git_url,omitempty"`
	S3Key        *string `json:"s3_key,omitempty"`
	LiveURL      *string `json:"live_url,omitempty"`
	ImageTag     *string `json:"image_tag,omitempty"`
	ErrorMessage *string `json:"error_message,omitempty"`
	CreatedAt    string  `json:"created_at"`
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
		ID:           d.ID,
		Status:       string(d.Status),
		Subdomain:    d.Subdomain,
		GitURL:       d.GitURL,
		S3Key:        d.S3Key,
		LiveURL:      d.LiveURL,
		ImageTag:     d.ImageTag,
		ErrorMessage: d.ErrorMessage,
		CreatedAt:    d.CreatedAt.Format(time.RFC3339),
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
