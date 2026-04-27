package builder

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/brimble/paas/entities"
	"github.com/brimble/paas/pkg/broker"
	"github.com/brimble/paas/pkg/logger"
)

type LogSink interface {
	InsertLog(ctx context.Context, l *entities.DeploymentLog) error
}

type BuildInfo struct {
	DetectedLang string `json:"detected_lang"`
	StartCmd     string `json:"start_cmd"`
}

type Service struct {
	cfg    buildConfig
	broker broker.LogPublisher
	sink   LogSink
	runner commandRunner
}

type buildConfig struct {
	mode string
}

type command interface {
	StdoutPipe() (io.ReadCloser, error)
	StderrPipe() (io.ReadCloser, error)
	Start() error
	Wait() error
}

type commandRunner interface {
	CommandContext(ctx context.Context, name string, args ...string) command
	LookPath(file string) (string, error)
}

type execRunner struct{}

type execCommand struct {
	*exec.Cmd
}

func NewService(mode string, b broker.LogPublisher, sink LogSink) *Service {
	return &Service{
		cfg:    buildConfig{mode: NormalizeMode(mode)},
		broker: b,
		sink:   sink,
		runner: execRunner{},
	}
}

func (r execRunner) CommandContext(ctx context.Context, name string, args ...string) command {
	return execCommand{Cmd: exec.CommandContext(ctx, name, args...)}
}

func (r execRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (s *Service) Mode() string {
	return s.cfg.mode
}

func (s *Service) Clone(ctx context.Context, gitURL, destDir, deploymentID string) error {
	logger.Info("cloning repository", "deploymentID", deploymentID, "gitURL", gitURL)
	cmd := s.runner.CommandContext(ctx, "git", "clone", "--depth", "1", gitURL, destDir)
	if err := s.streamCommand(ctx, cmd, deploymentID, "clone"); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}
	logger.Info("clone complete", "deploymentID", deploymentID)
	return nil
}

func (s *Service) Build(ctx context.Context, sourceDir, imageTag, deploymentID string) (*BuildInfo, error) {
	switch s.cfg.mode {
	case "prod":
		return s.buildProd(ctx, sourceDir, imageTag, deploymentID)
	default:
		return s.buildDev(ctx, sourceDir, imageTag, deploymentID)
	}
}

func (s *Service) buildDev(ctx context.Context, sourceDir, imageTag, deploymentID string) (*BuildInfo, error) {
	logger.Info("building (dev mode)", "deploymentID", deploymentID, "sourceDir", sourceDir, "imageTag", imageTag)

	cmd := s.runner.CommandContext(ctx, "railpack", "build", sourceDir, "--name", imageTag)
	if err := s.streamCommand(ctx, cmd, deploymentID, "build"); err != nil {
		return nil, fmt.Errorf("railpack build failed: %w", err)
	}

	logger.Info("build complete (dev mode)", "deploymentID", deploymentID, "imageTag", imageTag)
	return &BuildInfo{}, nil
}

func (s *Service) buildProd(ctx context.Context, sourceDir, imageTag, deploymentID string) (*BuildInfo, error) {
	logger.Info("building (prod mode)", "deploymentID", deploymentID, "sourceDir", sourceDir, "imageTag", imageTag)

	planPath := filepath.Join(sourceDir, "railpack-plan.json")
	infoPath := filepath.Join(sourceDir, "railpack-info.json")

	prepareCmd := s.runner.CommandContext(ctx, "railpack", "prepare", sourceDir,
		"--plan-out", planPath,
		"--info-out", infoPath,
	)
	if err := s.streamCommand(ctx, prepareCmd, deploymentID, "prepare"); err != nil {
		return nil, fmt.Errorf("railpack prepare failed: %w", err)
	}

	info, err := parseBuildInfo(infoPath)
	if err != nil {
		logger.Error(err, "failed to parse railpack-info.json, continuing without metadata", "deploymentID", deploymentID)
		info = &BuildInfo{}
	}

	buildCmd := s.runner.CommandContext(ctx,
		"docker", "buildx", "build",
		"--build-arg", "BUILDKIT_SYNTAX=ghcr.io/railwayapp/railpack-frontend",
		"-f", planPath,
		"-t", imageTag,
		"--load",
		sourceDir,
	)
	if err := s.streamCommand(ctx, buildCmd, deploymentID, "build"); err != nil {
		return nil, fmt.Errorf("docker buildx build failed: %w", err)
	}

	logger.Info("build complete (prod mode)", "deploymentID", deploymentID, "imageTag", imageTag,
		"detectedLang", info.DetectedLang, "startCmd", info.StartCmd)
	return info, nil
}

func (s *Service) streamCommand(ctx context.Context, cmd command, deploymentID, phase string) error {
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	done := make(chan struct{}, 2)

	go func() {
		defer func() { done <- struct{}{} }()
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			s.publishLine(ctx, deploymentID, phase, "stdout", scanner.Text())
		}
	}()

	go func() {
		defer func() { done <- struct{}{} }()
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			s.publishLine(ctx, deploymentID, phase, "stderr", scanner.Text())
		}
	}()

	<-done
	<-done

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("command exited with error: %w", err)
	}

	return nil
}

func (s *Service) publishLine(ctx context.Context, deploymentID, phase, stream, content string) {
	log := &entities.DeploymentLog{
		DeploymentID: deploymentID,
		Stream:       stream,
		Phase:        phase,
		Content:      content,
	}
	if err := s.sink.InsertLog(ctx, log); err != nil {
		logger.Error(err, "failed to persist log line", "deploymentID", deploymentID, "phase", phase)
		return
	}
	s.broker.Publish(deploymentID, broker.LogLine{
		ID:           log.ID,
		DeploymentID: log.DeploymentID,
		Timestamp:    log.Timestamp.Format(time.RFC3339),
		Phase:        log.Phase,
		Stream:       log.Stream,
		Content:      log.Content,
	})
}

func parseBuildInfo(path string) (*BuildInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read info file: %w", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse info JSON: %w", err)
	}

	info := &BuildInfo{}

	if provider, ok := raw["provider"].(map[string]any); ok {
		if name, ok := provider["name"].(string); ok {
			info.DetectedLang = name
		}
	}

	if startCmd, ok := raw["startCmd"].(string); ok {
		info.StartCmd = startCmd
	}

	if v, ok := raw["name"].(string); ok && info.DetectedLang == "" {
		info.DetectedLang = v
	}

	return info, nil
}

func CleanSourceDir(sourceDir string) {
	patterns := []string{"railpack-plan.json", "railpack-info.json"}
	for _, p := range patterns {
		path := filepath.Join(sourceDir, p)
		if _, err := os.Stat(path); err == nil {
			os.Remove(path)
		}
	}
}

func (s *Service) Validate() error {
	if _, err := s.runner.LookPath("git"); err != nil {
		return fmt.Errorf("git not found in PATH: %w", err)
	}
	if _, err := s.runner.LookPath("railpack"); err != nil {
		return fmt.Errorf("railpack CLI not found in PATH: %w", err)
	}
	if s.cfg.mode == "prod" {
		if _, err := s.runner.LookPath("docker"); err != nil {
			return fmt.Errorf("docker CLI not found in PATH (required for prod build mode): %w", err)
		}
	}
	return nil
}

func (s *Service) String() string {
	switch s.cfg.mode {
	case "prod":
		return "production (railpack prepare + docker buildx build)"
	default:
		return "development (railpack build)"
	}
}

func (s *Service) IsDevMode() bool {
	return s.cfg.mode != "prod"
}

func (s *Service) IsProdMode() bool {
	return s.cfg.mode == "prod"
}

func NormalizeMode(mode string) string {
	switch strings.ToLower(mode) {
	case "prod", "production":
		return "prod"
	default:
		return "dev"
	}
}
