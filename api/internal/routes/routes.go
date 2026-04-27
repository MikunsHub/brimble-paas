package routes

import (
	"github.com/brimble/paas/internal/deployment"
	"github.com/brimble/paas/pkg/handler"
	"github.com/gin-gonic/gin"
)

type Deps struct {
	Base          *handler.BaseHandler
	DeploymentSvc deployment.Service
}

func Register(r *gin.Engine, deps Deps) {
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	deployHandler := deployment.NewDeploymentHandler(deps.Base, deps.DeploymentSvc)

	api := r.Group("/api")
	{
		deploys := api.Group("/deployments")
		{
			deploys.POST("/upload-url", deployHandler.CreateUploadURL)
			deploys.POST("", deployHandler.Create)
			deploys.GET("", deployHandler.List)
			deploys.GET("/:id", deployHandler.Get)
			deploys.DELETE("/:id", deployHandler.Delete)
			deploys.POST("/:id/restart", deployHandler.Restart)
			deploys.GET("/:id/logs", deployHandler.GetLogs)
			deploys.GET("/:id/logs/stream", deployHandler.StreamLogs)
		}
	}
}
