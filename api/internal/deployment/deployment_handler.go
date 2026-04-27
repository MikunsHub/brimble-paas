package deployment

import (
	"fmt"
	"net/http"
	"strconv"

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
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil {
		h.BadRequest(c, "offset must be a valid integer")
		return
	}

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
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil {
		h.BadRequest(c, "offset must be a valid integer")
		return
	}

	session, err := h.svc.OpenLogStream(c.Request.Context(), c.Param("id"), offset)
	if err != nil {
		h.HandleErr(c, err)
		return
	}
	defer session.Close()

	h.prepareSSE(c)
	h.emitDeploymentStatus(c, session.InitialStatus)

	for _, log := range session.History {
		h.emitLogEvent(c, log)
	}

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case log, ok := <-session.LiveLogs:
			if !ok {
				return
			}
			h.emitLogEvent(c, log)
		case status, ok := <-session.StatusUpdates:
			if !ok {
				return
			}
			h.emitDeploymentStatus(c, status)
		}
	}
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

func (h *Handler) emitDeploymentStatus(c *gin.Context, status StreamStatusEvent) {
	payload := gin.H{
		"status": status.Status,
	}
	if status.LiveURL != "" {
		payload["live_url"] = status.LiveURL
	}
	if status.ErrorMessage != "" {
		payload["error_message"] = status.ErrorMessage
	}
	c.SSEvent("status", payload)
	c.Writer.Flush()
}

func (h *Handler) emitLogEvent(c *gin.Context, event StreamLogEvent) {
	fmt.Fprintf(c.Writer, "id: %d\n", event.Index)
	c.SSEvent("log", event)
	c.Writer.Flush()
}
