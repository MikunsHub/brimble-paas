package deployment

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/brimble/paas/entities"
	"github.com/brimble/paas/pkg/broker"
	"github.com/brimble/paas/pkg/handler"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	*handler.BaseHandler
	svc Service
}

func NewDeploymentHandler(base *handler.BaseHandler, svc Service) *Handler {
	return &Handler{BaseHandler: base, svc: svc}
}

func (h *Handler) Create(c *gin.Context) {
	var req CreateDeploymentRequest
	if !h.BindJSON(c, &req) {
		return
	}

	d, err := h.svc.Create(c.Request.Context(), req)
	if err != nil {
		h.HandleErr(c, err)
		return
	}

	h.Created(c, "deployment created", toDeploymentResponse(d))
}

func (h *Handler) CreateUploadURL(c *gin.Context) {
	var req CreateUploadURLRequest
	if !h.BindJSON(c, &req) {
		return
	}

	resp, err := h.svc.CreateUploadURL(c.Request.Context(), req)
	if err != nil {
		h.HandleErr(c, err)
		return
	}

	h.Created(c, "upload url created", resp)
}

func (h *Handler) List(c *gin.Context) {
	deployments, err := h.svc.List(c.Request.Context())
	if err != nil {
		h.HandleErr(c, err)
		return
	}

	resp := make([]DeploymentResponse, len(deployments))
	for i, d := range deployments {
		resp[i] = toDeploymentResponse(d)
	}

	h.OK(c, "deployments fetched", resp)
}

func (h *Handler) Get(c *gin.Context) {
	d, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.HandleErr(c, err)
		return
	}

	h.OK(c, "deployment fetched", toDeploymentResponse(d))
}

func (h *Handler) Delete(c *gin.Context) {
	if err := h.svc.Teardown(c.Request.Context(), c.Param("id")); err != nil {
		h.HandleErr(c, err)
		return
	}

	h.OK(c, "deployment stopped", nil)
}

func (h *Handler) Restart(c *gin.Context) {
	d, err := h.svc.Restart(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.HandleErr(c, err)
		return
	}

	h.OK(c, "deployment restarted", toDeploymentResponse(d))
}

func (h *Handler) GetLogs(c *gin.Context) {
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	logs, err := h.svc.GetLogs(c.Request.Context(), c.Param("id"), offset)
	if err != nil {
		h.HandleErr(c, err)
		return
	}

	resp := make([]LogResponse, len(logs))
	for i, l := range logs {
		resp[i] = toLogResponse(l)
	}

	h.OK(c, "logs fetched", resp)
}

func (h *Handler) StreamLogs(c *gin.Context) {
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if offset < 0 {
		h.BadRequest(c, "offset must be greater than or equal to 0")
		return
	}

	deploymentID := c.Param("id")
	deployment, err := h.svc.Get(c.Request.Context(), deploymentID)
	if err != nil {
		h.HandleErr(c, err)
		return
	}

	history, err := h.svc.GetLogs(c.Request.Context(), deploymentID, offset)
	if err != nil {
		h.HandleErr(c, err)
		return
	}

	liveCh, unsubscribe, err := h.svc.SubscribeLogs(c.Request.Context(), deploymentID)
	if err != nil {
		h.HandleErr(c, err)
		return
	}
	defer unsubscribe()

	catchup, err := h.svc.GetLogs(c.Request.Context(), deploymentID, offset+len(history))
	if err != nil {
		h.HandleErr(c, err)
		return
	}

	h.prepareSSE(c)
	h.emitDeploymentStatus(c, deployment)

	nextIndex := offset
	for _, log := range history {
		h.emitLogEvent(c, deploymentLogToStreamEvent(log, nextIndex))
		nextIndex++
	}
	for _, log := range catchup {
		h.emitLogEvent(c, deploymentLogToStreamEvent(log, nextIndex))
		nextIndex++
	}

	// Live lines published between subscription and the catch-up query are
	// already covered by catch-up history, so drop them once from the broker queue.
	pendingDuplicates := len(catchup)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	lastStatus := deployment.Status
	lastLiveURL := stringPtrValue(deployment.LiveURL)
	lastError := stringPtrValue(deployment.ErrorMessage)

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case line, ok := <-liveCh:
			if !ok {
				return
			}
			if pendingDuplicates > 0 {
				pendingDuplicates--
				continue
			}
			h.emitLogEvent(c, logLineToStreamEvent(deploymentID, line, nextIndex))
			nextIndex++
		case <-ticker.C:
			current, err := h.svc.Get(c.Request.Context(), deploymentID)
			if err != nil {
				// Client already has history/log stream; surface backend failure as SSE.
				c.SSEvent("error", gin.H{"message": "failed to refresh deployment status"})
				c.Writer.Flush()
				return
			}

			currentLiveURL := stringPtrValue(current.LiveURL)
			currentError := stringPtrValue(current.ErrorMessage)
			if current.Status != lastStatus || currentLiveURL != lastLiveURL || currentError != lastError {
				h.emitDeploymentStatus(c, current)
				lastStatus = current.Status
				lastLiveURL = currentLiveURL
				lastError = currentError
			}

			if isTerminalStatus(current.Status) {
				return
			}
		}
	}
}

type streamLogEvent struct {
	Index        int    `json:"index"`
	ID           string `json:"id,omitempty"`
	DeploymentID string `json:"deployment_id,omitempty"`
	Timestamp    string `json:"timestamp,omitempty"`
	Stream       string `json:"stream"`
	Phase        string `json:"phase"`
	Content      string `json:"content"`
}

func (h *Handler) prepareSSE(c *gin.Context) {
	header := c.Writer.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	c.Writer.Flush()
}

func (h *Handler) emitDeploymentStatus(c *gin.Context, d *entities.Deployment) {
	payload := gin.H{
		"status": d.Status,
	}
	if d.LiveURL != nil {
		payload["live_url"] = *d.LiveURL
	}
	if d.ErrorMessage != nil {
		payload["error_message"] = *d.ErrorMessage
	}
	c.SSEvent("status", payload)
	c.Writer.Flush()
}

func (h *Handler) emitLogEvent(c *gin.Context, event streamLogEvent) {
	c.Writer.Write([]byte(fmt.Sprintf("id: %d\n", event.Index)))
	c.SSEvent("log", event)
	c.Writer.Flush()
}

func deploymentLogToStreamEvent(log *entities.DeploymentLog, index int) streamLogEvent {
	return streamLogEvent{
		Index:        index,
		ID:           log.ID,
		DeploymentID: log.DeploymentID,
		Timestamp:    log.Timestamp.Format(time.RFC3339),
		Stream:       log.Stream,
		Phase:        log.Phase,
		Content:      log.Content,
	}
}

func logLineToStreamEvent(_ string, line broker.LogLine, index int) streamLogEvent {
	return streamLogEvent{
		Index:        index,
		ID:           line.ID,
		DeploymentID: line.DeploymentID,
		Timestamp:    line.Timestamp,
		Stream:       line.Stream,
		Phase:        line.Phase,
		Content:      line.Content,
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
