package deployment

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
