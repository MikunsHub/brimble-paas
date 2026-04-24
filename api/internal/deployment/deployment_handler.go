package deployment

import (
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
	// TODO: SSE streaming — subscribe to broker, replay Postgres history from offset,
	// then stream live via c.Stream() + c.SSEvent()
	h.InternalError(c, "not implemented")
}
