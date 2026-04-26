package entities

import "time"

type DeploymentStatus string

const (
	StatusPending   DeploymentStatus = "pending"
	StatusBuilding  DeploymentStatus = "building"
	StatusDeploying DeploymentStatus = "deploying"
	StatusRunning   DeploymentStatus = "running"
	StatusFailed    DeploymentStatus = "failed"
	StatusStopped   DeploymentStatus = "stopped"
)

type Deployment struct {
	ID            string           `json:"id"             gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	GitURL        *string          `json:"git_url"        gorm:"column:git_url"`
	S3Key         *string          `json:"s3_key"         gorm:"column:s3_key"`
	Subdomain     string           `json:"subdomain"      gorm:"uniqueIndex;not null"`
	Status        DeploymentStatus `json:"status"         gorm:"not null;default:'pending'"`
	ImageTag      *string          `json:"image_tag"      gorm:"column:image_tag"`
	ContainerID   *string          `json:"container_id"   gorm:"column:container_id"`
	ContainerAddr *string          `json:"container_addr" gorm:"column:container_addr"`
	LiveURL       *string          `json:"live_url"       gorm:"column:live_url"`
	DetectedLang  *string          `json:"detected_lang"  gorm:"column:detected_lang"`
	StartCmd      *string          `json:"start_cmd"      gorm:"column:start_cmd"`
	ErrorMessage  *string          `json:"error_message"  gorm:"column:error_message"`
	CreatedAt     time.Time        `json:"created_at"     gorm:"autoCreateTime"`
	UpdatedAt     time.Time        `json:"updated_at"     gorm:"autoUpdateTime"`
}

type DeploymentLog struct {
	ID           string    `json:"id"            gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	DeploymentID string    `json:"deployment_id" gorm:"type:uuid;not null;index"`
	Timestamp    time.Time `json:"timestamp"     gorm:"autoCreateTime"`
	Stream       string    `json:"stream"        gorm:"not null"` // stdout | stderr
	Phase        string    `json:"phase"         gorm:"not null"` // clone | build | deploy | health
	Content      string    `json:"content"       gorm:"not null"`
}
