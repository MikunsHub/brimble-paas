package main

import (
	"context"
	"os"
	"strings"

	"github.com/brimble/paas/config"
	"github.com/brimble/paas/internal/builder"
	"github.com/brimble/paas/internal/caddy"
	"github.com/brimble/paas/internal/deployment"
	"github.com/brimble/paas/internal/docker"
	"github.com/brimble/paas/internal/routes"
	s3client "github.com/brimble/paas/pkg/aws/s3"
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

	dockerSvc, err := docker.NewDockerService(cfg.DockerHost, cfg.DockerNetwork)
	if err != nil {
		logger.Error(err, "failed to create docker service")
		os.Exit(1)
	}

	caddySvc := caddy.NewCaddyService(cfg.CaddyAdminURL, cfg.Domain)
	s3, err := s3client.NewClient(
		context.Background(),
		cfg.AWSRegion,
		cfg.AWSEndpointURL,
		cfg.AWSAccessKeyID,
		cfg.AWSSecretKey,
		cfg.S3Bucket,
	)
	if err != nil {
		logger.Error(err, "failed to create s3 client")
		os.Exit(1)
	}
	if err := s3.EnsureBucket(context.Background()); err != nil {
		logger.Error(err, "failed to ensure source bucket")
		os.Exit(1)
	}

	deploymentSvc := deployment.NewDeploymentService(
		deploymentRepo,
		logBroker,
		builderSvc,
		dockerSvc,
		caddySvc,
		s3,
		cfg,
	)

	base := &handler.BaseHandler{
		DB:     db,
		Logger: appLogger,
	}

	gin.SetMode(gin.ReleaseMode)

	allowedOrigins := map[string]bool{}
	for _, o := range strings.Split(cfg.AllowedOrigins, ",") {
		if o = strings.TrimSpace(o); o != "" {
			allowedOrigins[o] = true
		}
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(logger.GinMiddleware(appLogger, cfg.Env))
	r.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if allowedOrigins[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type")
			c.Header("Access-Control-Max-Age", "86400")
		}
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})
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
