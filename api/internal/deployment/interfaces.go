package deployment

import (
	"context"
	"io"
	"time"

	"github.com/brimble/paas/internal/deployment/builder"
	"github.com/docker/docker/api/types/container"
)

type Builder interface {
	Clone(ctx context.Context, gitURL, destDir, deploymentID string) error
	Build(ctx context.Context, sourceDir, imageTag, deploymentID string) (*builder.BuildInfo, error)
}

type DockerManager interface {
	RunContainer(ctx context.Context, imageTag, subdomain string) (containerID, containerAddr string, err error)
	StopContainer(ctx context.Context, containerID string) error
	InspectContainer(ctx context.Context, containerID string) (*container.State, error)
	WaitForHealthy(ctx context.Context, containerID string, timeout time.Duration) error
	GetContainerLogs(ctx context.Context, containerID string) (string, error)
	StreamContainerLogs(ctx context.Context, containerID string) (io.ReadCloser, error)
}

type Router interface {
	AddRoute(ctx context.Context, subdomain, upstreamAddr string) error
	RemoveRoute(ctx context.Context, subdomain string) error
}
