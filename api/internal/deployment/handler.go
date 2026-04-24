package deployment

import (
	"github.com/brimble/paas/pkg/handler"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	*handler.BaseHandler
	svc Service
}

func NewHandler(base *handler.BaseHandler, svc Service) *Handler {
	return &Handler{BaseHandler: base, svc: svc}
}

func (h *Handler) Create(c *gin.Context) {
	h.InternalError(c, "not implemented")
}

func (h *Handler) List(c *gin.Context) {
	h.InternalError(c, "not implemented")
}

func (h *Handler) Get(c *gin.Context) {
	h.InternalError(c, "not implemented")
}

func (h *Handler) Delete(c *gin.Context) {
	h.InternalError(c, "not implemented")
}

func (h *Handler) GetLogs(c *gin.Context) {
	h.InternalError(c, "not implemented")
}

func (h *Handler) StreamLogs(c *gin.Context) {
	h.InternalError(c, "not implemented")
}
