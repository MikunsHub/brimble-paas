package broker

// LogPublisher is the interface all broker implementations satisfy.
// Swap via LOG_BROKER env var: channel (default) | redis | nats
type LogPublisher interface {
	Publish(deploymentID string, line LogLine) error
	Subscribe(deploymentID string) (<-chan LogLine, func(), error)
}

// LogLine is a single log entry streamed from a build/deploy phase.
type LogLine struct {
	Index        int    `json:"index"`
	ID           string `json:"id,omitempty"`
	DeploymentID string `json:"deployment_id,omitempty"`
	Timestamp    string `json:"timestamp,omitempty"`
	Phase        string `json:"phase"`  // clone | build | deploy | health
	Stream       string `json:"stream"` // stdout | stderr
	Content      string `json:"content"`
}
