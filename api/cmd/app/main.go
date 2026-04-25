package main

import (
	"os"

	"github.com/brimble/paas/config"
	"github.com/brimble/paas/internal/builder"
	"github.com/brimble/paas/internal/deployment"
	"github.com/brimble/paas/internal/routes"
	"github.com/brimble/paas/pkg/broker"
	"github.com/brimble/paas/pkg/handler"
	"github.com/brimble/paas/pkg/logger"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()

	appLogger := logger.Initialize(cfg.Env)
	logger.Info("starting brimble-paas api", "port", cfg.Port, "env", cfg.Env)

	db, err := config.NewDB(cfg)
	if err != nil {
		logger.Error(err, "failed to connect to database")
		os.Exit(1)
	}
	logger.Info("database connection established")

	logBroker := broker.NewChannelBroker()

	deploymentRepo := deployment.NewDeploymentRepo(db)

	builderSvc := builder.NewBuilderService(cfg.BuildMode, logBroker, deploymentRepo)
	if err := builderSvc.Validate(); err != nil {
		logger.Error(err, "builder validation failed")
		os.Exit(1)
	}
	logger.Info("builder ready", "mode", builderSvc.String())

	deploymentSvc := deployment.NewDeploymentService(deploymentRepo, logBroker, builderSvc, cfg)

	base := &handler.BaseHandler{
		DB:     db,
		Logger: appLogger,
	}

	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(logger.GinMiddleware(appLogger, cfg.Env))
	r.SetTrustedProxies(nil)

	routes.Register(r, routes.Deps{
		Base:          base,
		DeploymentSvc: deploymentSvc,
	})

	logger.Info("server listening", "port", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		logger.Error(err, "server stopped unexpectedly")
		os.Exit(1)
	}
}
