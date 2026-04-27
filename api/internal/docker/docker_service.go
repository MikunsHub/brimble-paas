package docker

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

const appPort = "5000"

type DockerService struct {
	client  *client.Client
	network string
}

func NewDockerService(host, dockerNetwork string) (*DockerService, error) {
	opts := []client.Opt{client.WithAPIVersionNegotiation()}
	if host != "" {
		opts = append(opts, client.WithHost(host))
	}
	c, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}
	return &DockerService{client: c, network: dockerNetwork}, nil
}

func (s *DockerService) RunContainer(ctx context.Context, imageTag, subdomain string) (containerID, containerAddr string, err error) {
	name := "brimble-" + subdomain

	resp, err := s.client.ContainerCreate(ctx,
		&container.Config{
			Image: imageTag,
			Env:   []string{"PORT=" + appPort}, // maybe support environment variables to start server
		},
		&container.HostConfig{
			RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
		},
		&network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				s.network: {},
			},
		},
		nil,
		name,
	)
	if err != nil {
		return "", "", fmt.Errorf("failed to create container: %w", err)
	}

	if err := s.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", "", fmt.Errorf("failed to start container: %w", err)
	}

	info, err := s.client.ContainerInspect(ctx, resp.ID)
	if err != nil {
		return "", "", fmt.Errorf("failed to inspect container: %w", err)
	}

	netSettings, ok := info.NetworkSettings.Networks[s.network]
	if !ok || netSettings.IPAddress == "" {
		return "", "", fmt.Errorf("container not connected to network %q", s.network)
	}

	return resp.ID, netSettings.IPAddress + ":" + appPort, nil
}

func (s *DockerService) StopContainer(ctx context.Context, containerID string) error {
	timeout := 10
	if err := s.client.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout}); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}
	if err := s.client.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}
	return nil
}

func (s *DockerService) InspectContainer(ctx context.Context, containerID string) (*container.State, error) {
	info, err := s.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}
	return info.State, nil
}

func (s *DockerService) WaitForHealthy(ctx context.Context, containerID string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	stableThreshold := 1 * time.Second
	var stableSince time.Time
	var lastAppAddr string

	for {
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				if lastAppAddr != "" {
					return fmt.Errorf("app did not accept TCP connections on %s after %v", lastAppAddr, timeout)
				}
				return fmt.Errorf("health check timed out after %v", timeout)
			}
			return ctx.Err()
		case <-ticker.C:
			info, err := s.client.ContainerInspect(ctx, containerID)
			if err != nil {
				if ctx.Err() == context.DeadlineExceeded {
					if lastAppAddr != "" {
						return fmt.Errorf("app did not accept TCP connections on %s after %v", lastAppAddr, timeout)
					}
					return fmt.Errorf("health check timed out after %v", timeout)
				}
				return fmt.Errorf("failed to inspect container: %w", err)
			}

			if !info.State.Running && info.State.ExitCode != 0 {
				return fmt.Errorf("container exited with code %d", info.State.ExitCode)
			}

			if info.RestartCount > 0 {
				return fmt.Errorf("container is unstable (restart count: %d)", info.RestartCount)
			}

			if info.State.Running && info.RestartCount == 0 {
				appAddr, err := s.containerAppAddress(info.NetworkSettings)
				if err != nil {
					return err
				}
				lastAppAddr = appAddr
				if !tcpReachable(appAddr, 300*time.Millisecond) {
					stableSince = time.Time{}
					continue
				}
				if stableSince.IsZero() {
					stableSince = time.Now()
				} else if time.Since(stableSince) >= stableThreshold {
					return nil
				}
			} else {
				stableSince = time.Time{}
			}
		}
	}
}

func (s *DockerService) containerAppAddress(settings *container.NetworkSettings) (string, error) {
	if settings == nil {
		return "", fmt.Errorf("container network settings unavailable")
	}
	netSettings, ok := settings.Networks[s.network]
	if !ok || netSettings.IPAddress == "" {
		return "", fmt.Errorf("container not connected to network %q", s.network)
	}
	return net.JoinHostPort(netSettings.IPAddress, appPort), nil
}

func tcpReachable(addr string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func (s *DockerService) GetContainerLogs(ctx context.Context, containerID string) (string, error) {
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       "100",
	}
	logs, err := s.client.ContainerLogs(ctx, containerID, options)
	if err != nil {
		return "", fmt.Errorf("failed to get container logs: %w", err)
	}
	defer logs.Close()

	var stdout, stderr strings.Builder
	if _, err := stdcopy.StdCopy(&stdout, &stderr, logs); err != nil {
		return "", fmt.Errorf("failed to read container logs: %w", err)
	}

	result := stdout.String()
	if stderr.Len() > 0 {
		if result != "" {
			result += "\n"
		}
		result += "STDERR:\n" + stderr.String()
	}
	return result, nil
}

func (s *DockerService) StreamContainerLogs(ctx context.Context, containerID string) (io.ReadCloser, error) {
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Tail:       "0",
	}
	rc, err := s.client.ContainerLogs(ctx, containerID, options)
	if err != nil {
		return nil, fmt.Errorf("failed to stream container logs: %w", err)
	}
	return rc, nil
}

func (s *DockerService) Close() error {
	return s.client.Close()
}
