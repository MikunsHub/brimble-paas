package deployment

import (
	"context"
	"time"

	"github.com/brimble/paas/entities"
	"github.com/brimble/paas/pkg/broker"
)

type StreamLogEvent struct {
	Index        int    `json:"index"`
	ID           string `json:"id,omitempty"`
	DeploymentID string `json:"deployment_id,omitempty"`
	Timestamp    string `json:"timestamp,omitempty"`
	Stream       string `json:"stream"`
	Phase        string `json:"phase"`
	Content      string `json:"content"`
}

type StreamStatusEvent struct {
	Status       entities.DeploymentStatus `json:"status"`
	LiveURL      string                    `json:"live_url,omitempty"`
	ErrorMessage string                    `json:"error_message,omitempty"`
}

type LogStreamSession struct {
	InitialStatus StreamStatusEvent
	History       []StreamLogEvent
	LiveLogs      <-chan StreamLogEvent
	StatusUpdates <-chan StreamStatusEvent
	Close         func()
}

type runtimeLogWriter struct {
	deploymentID string
	phase        string
	stream       string
	broker       broker.LogPublisher
	repo         Repository
	ctx          context.Context
}

func deploymentLogToStreamEvent(log *entities.DeploymentLog, index int) StreamLogEvent {
	return StreamLogEvent{
		Index:        index,
		ID:           log.ID,
		DeploymentID: log.DeploymentID,
		Timestamp:    log.Timestamp.Format(time.RFC3339),
		Stream:       log.Stream,
		Phase:        log.Phase,
		Content:      log.Content,
	}
}

func brokerLogToStreamEvent(line broker.LogLine, index int) StreamLogEvent {
	return StreamLogEvent{
		Index:        index,
		ID:           line.ID,
		DeploymentID: line.DeploymentID,
		Timestamp:    line.Timestamp,
		Stream:       line.Stream,
		Phase:        line.Phase,
		Content:      line.Content,
	}
}

func deploymentToStatusEvent(d *entities.Deployment) StreamStatusEvent {
	return StreamStatusEvent{
		Status:       d.Status,
		LiveURL:      stringPtrValue(d.LiveURL),
		ErrorMessage: stringPtrValue(d.ErrorMessage),
	}
}

func stringPtrValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func isTerminalStatus(status entities.DeploymentStatus) bool {
	switch status {
	case entities.StatusFailed, entities.StatusStopped:
		return true
	default:
		return false
	}
}
