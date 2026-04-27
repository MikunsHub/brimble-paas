package deployment

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/brimble/paas/config"
	"github.com/brimble/paas/entities"
	"github.com/brimble/paas/pkg/broker"
	apperrors "github.com/brimble/paas/pkg/errors"
	"github.com/brimble/paas/pkg/logger"
	"github.com/docker/docker/pkg/stdcopy"
)

type Service interface {
	Create(ctx context.Context, req CreateDeploymentRequest) (*entities.Deployment, error)
	CreateUploadURL(ctx context.Context, req CreateUploadURLRequest) (*CreateUploadURLResponse, error)
	Get(ctx context.Context, id string) (*entities.Deployment, error)
	List(ctx context.Context) ([]*entities.Deployment, error)
	Teardown(ctx context.Context, id string) error
	Restart(ctx context.Context, id string) (*entities.Deployment, error)
	GetLogs(ctx context.Context, id string, offset int) ([]*entities.DeploymentLog, error)
	OpenLogStream(ctx context.Context, id string, offset int) (*LogStreamSession, error)
}

type deploymentService struct {
	repo       Repository
	broker     broker.LogPublisher
	builderSvc Builder
	dockerSvc  DockerManager
	caddySvc   Router
	s3         s3API
	cfg        *config.Config
}

type s3API interface {
	CreateDeploymentUploadURL(ctx context.Context, fileName, contentType string, expires time.Duration) (filePath, url string, err error)
	Upload(ctx context.Context, key string, body io.Reader, contentType string) error
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	Exists(ctx context.Context, key string) (bool, error)
}

func NewDeploymentService(
	repo Repository,
	b broker.LogPublisher,
	builderSvc Builder,
	dockerSvc DockerManager,
	caddySvc Router,
	s3 s3API,
	cfg *config.Config,
) Service {
	return &deploymentService{
		repo:       repo,
		broker:     b,
		builderSvc: builderSvc,
		dockerSvc:  dockerSvc,
		caddySvc:   caddySvc,
		s3:         s3,
		cfg:        cfg,
	}
}

func (s *deploymentService) Create(ctx context.Context, req CreateDeploymentRequest) (*entities.Deployment, error) {
	hasGitURL := req.GitURL != nil && strings.TrimSpace(*req.GitURL) != ""
	hasFilePath := req.FilePath != nil && strings.TrimSpace(*req.FilePath) != ""
	if hasGitURL == hasFilePath {
		return nil, apperrors.NewBadRequestError("provide exactly one of git_url or file_path")
	}

	d := &entities.Deployment{
		Subdomain: GenerateSubDomainSlug(),
		Status:    entities.StatusPending,
	}
	if hasGitURL {
		gitURL := strings.TrimSpace(*req.GitURL)
		d.GitURL = &gitURL
	} else {
		filePath := strings.TrimSpace(*req.FilePath)
		exists, err := s.s3.Exists(ctx, filePath)
		if err != nil {
			return nil, apperrors.NewInternalError("failed to validate file path")
		}
		if !exists {
			return nil, apperrors.NewBadRequestError("file_path does not exist")
		}
		d.S3Key = &filePath
	}

	if err := s.repo.Create(ctx, d); err != nil {
		return nil, apperrors.NewInternalError("failed to create deployment")
	}

	logger.Info("deployment created", "id", d.ID, "subdomain", d.Subdomain)

	go s.runPipeline(cloneDeployment(d))

	return d, nil
}

func (s *deploymentService) CreateUploadURL(ctx context.Context, req CreateUploadURLRequest) (*CreateUploadURLResponse, error) {
	fileName := strings.TrimSpace(req.FileName)
	if fileName == "" {
		return nil, apperrors.NewBadRequestError("file_name is required")
	}

	contentType := strings.TrimSpace(req.ContentType)
	filePath, url, err := s.s3.CreateDeploymentUploadURL(ctx, fileName, contentType, 15*time.Minute)
	if err != nil {
		if strings.Contains(err.Error(), ".zip") {
			return nil, apperrors.NewBadRequestError("file must be a .zip archive")
		}
		return nil, apperrors.NewInternalError("failed to create upload url")
	}

	return &CreateUploadURLResponse{
		FilePath: filePath,
		URL:      url,
		Method:   "PUT",
	}, nil
}

func (s *deploymentService) Get(ctx context.Context, id string) (*entities.Deployment, error) {
	d, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, apperrors.NewInternalError("failed to fetch deployment")
	}
	if d == nil {
		return nil, apperrors.NewNotFoundError("deployment not found")
	}
	return d, nil
}

func (s *deploymentService) List(ctx context.Context) ([]*entities.Deployment, error) {
	deployments, err := s.repo.List(ctx)
	if err != nil {
		return nil, apperrors.NewInternalError("failed to list deployments")
	}
	return deployments, nil
}

func (s *deploymentService) Teardown(ctx context.Context, id string) error {
	d, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return apperrors.NewInternalError("failed to fetch deployment")
	}
	if d == nil {
		return apperrors.NewNotFoundError("deployment not found")
	}

	s.publishLogLines(ctx, id, "teardown", "stdout", "Stopping deployment...")

	if d.ContainerID != nil {
		short := (*d.ContainerID)[:12]
		s.publishLogLines(ctx, id, "teardown", "stdout", "Sending SIGTERM to container "+short+"...")
		if err := s.dockerSvc.StopContainer(ctx, *d.ContainerID); err != nil {
			logger.Error(err, "teardown: failed to stop container", "id", id, "containerID", *d.ContainerID)
			s.publishLogLines(ctx, id, "teardown", "stderr", "Warning: "+err.Error())
		} else {
			s.publishLogLines(ctx, id, "teardown", "stdout", "Container stopped successfully")
		}
	}

	if err := s.caddySvc.RemoveRoute(ctx, d.Subdomain); err != nil {
		logger.Error(err, "teardown: failed to remove caddy route", "id", id, "subdomain", d.Subdomain)
	} else {
		s.publishLogLines(ctx, id, "teardown", "stdout", "Route removed for "+d.Subdomain)
	}

	d.Status = entities.StatusStopped
	if err := s.repo.Update(ctx, d); err != nil {
		return apperrors.NewInternalError("failed to update deployment status")
	}

	s.publishLogLines(ctx, id, "teardown", "stdout", "Deployment stopped")
	logger.Info("deployment stopped", "id", id)
	return nil
}

func (s *deploymentService) Restart(ctx context.Context, id string) (*entities.Deployment, error) {
	d, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, apperrors.NewInternalError("failed to fetch deployment")
	}
	if d == nil {
		return nil, apperrors.NewNotFoundError("deployment not found")
	}
	if d.Status != entities.StatusStopped && d.Status != entities.StatusFailed {
		return nil, apperrors.NewBadRequestError("deployment is not in a restartable state")
	}
	if d.ImageTag == nil && d.GitURL == nil && d.S3Key == nil {
		return nil, apperrors.NewBadRequestError("no source available to restart from")
	}

	s.publishLogLines(ctx, id, "restart", "stdout", "--- Restarted ---")

	d.Status = entities.StatusPending
	d.ContainerID = nil
	d.ContainerAddr = nil
	d.ErrorMessage = nil

	if err := s.repo.Update(ctx, d); err != nil {
		return nil, apperrors.NewInternalError("failed to reset deployment")
	}

	if d.ImageTag != nil {
		go s.runFromImage(cloneDeployment(d))
	} else {
		go s.runPipeline(cloneDeployment(d))
	}

	logger.Info("deployment restart initiated", "id", id)
	return d, nil
}

func (s *deploymentService) GetLogs(ctx context.Context, id string, offset int) ([]*entities.DeploymentLog, error) {
	logs, err := s.repo.GetLogs(ctx, id, offset)
	if err != nil {
		return nil, apperrors.NewInternalError("failed to fetch logs")
	}
	return logs, nil
}

func (s *deploymentService) OpenLogStream(ctx context.Context, id string, offset int) (*LogStreamSession, error) {
	if offset < 0 {
		return nil, apperrors.NewBadRequestError("offset must be greater than or equal to 0")
	}

	d, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, apperrors.NewInternalError("failed to fetch deployment")
	}
	if d == nil {
		return nil, apperrors.NewNotFoundError("deployment not found")
	}

	history, err := s.repo.GetLogs(ctx, id, offset)
	if err != nil {
		return nil, apperrors.NewInternalError("failed to fetch logs")
	}

	liveSource, unsubscribe, err := s.broker.Subscribe(id)
	if err != nil {
		return nil, apperrors.NewInternalError("failed to subscribe to deployment logs")
	}

	catchup, err := s.repo.GetLogs(ctx, id, offset+len(history))
	if err != nil {
		unsubscribe()
		return nil, apperrors.NewInternalError("failed to fetch logs")
	}

	historyEvents := make([]StreamLogEvent, 0, len(history)+len(catchup))
	nextIndex := offset
	for _, log := range history {
		historyEvents = append(historyEvents, deploymentLogToStreamEvent(log, nextIndex))
		nextIndex++
	}
	for _, log := range catchup {
		historyEvents = append(historyEvents, deploymentLogToStreamEvent(log, nextIndex))
		nextIndex++
	}

	liveLogs := make(chan StreamLogEvent, 256)
	statusUpdates := make(chan StreamStatusEvent, 16)
	stop := make(chan struct{})

	go func() {
		defer unsubscribe()
		defer close(liveLogs)
		defer close(statusUpdates)

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		pendingDuplicates := len(catchup)
		lastStatus := d.Status
		lastLiveURL := stringPtrValue(d.LiveURL)
		lastError := stringPtrValue(d.ErrorMessage)

		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case line, ok := <-liveSource:
				if !ok {
					return
				}
				if pendingDuplicates > 0 {
					pendingDuplicates--
					continue
				}

				event := brokerLogToStreamEvent(line, nextIndex)
				nextIndex++

				select {
				case liveLogs <- event:
				case <-ctx.Done():
					return
				case <-stop:
					return
				}
			case <-ticker.C:
				current, err := s.repo.GetByID(ctx, id)
				if err != nil || current == nil {
					return
				}

				currentLiveURL := stringPtrValue(current.LiveURL)
				currentError := stringPtrValue(current.ErrorMessage)
				if current.Status != lastStatus || currentLiveURL != lastLiveURL || currentError != lastError {
					status := deploymentToStatusEvent(current)
					lastStatus = current.Status
					lastLiveURL = currentLiveURL
					lastError = currentError

					select {
					case statusUpdates <- status:
					case <-ctx.Done():
						return
					case <-stop:
						return
					}
				}

				if isTerminalStatus(current.Status) {
					return
				}
			}
		}
	}()

	return &LogStreamSession{
		InitialStatus: deploymentToStatusEvent(d),
		History:       historyEvents,
		LiveLogs:      liveLogs,
		StatusUpdates: statusUpdates,
		Close: func() {
			close(stop)
		},
	}, nil
}

func (s *deploymentService) runPipeline(d *entities.Deployment) {
	ctx := context.Background()
	logger.Info("pipeline started", "id", d.ID, "subdomain", d.Subdomain)

	d.Status = entities.StatusBuilding
	if err := s.repo.Update(ctx, d); err != nil {
		logger.Error(err, "pipeline: failed to update status to building", "id", d.ID)
	}

	sourceDir, err := os.MkdirTemp("", "brimble-"+d.ID+"-")
	if err != nil {
		s.failPipeline(ctx, d, fmt.Errorf("failed to create temp dir: %w", err))
		return
	}
	defer os.RemoveAll(sourceDir)

	if err := s.acquireSource(ctx, d, sourceDir); err != nil {
		s.failPipeline(ctx, d, err)
		return
	}

	imageTag := d.Subdomain + ":latest"
	info, err := s.builderSvc.Build(ctx, sourceDir, imageTag, d.ID)
	if err != nil {
		s.failPipeline(ctx, d, fmt.Errorf("build: %w", err))
		return
	}

	d.ImageTag = &imageTag
	if info.DetectedLang != "" {
		d.DetectedLang = &info.DetectedLang
	}
	if info.StartCmd != "" {
		d.StartCmd = &info.StartCmd
	}

	d.Status = entities.StatusDeploying
	if err := s.repo.Update(ctx, d); err != nil {
		logger.Error(err, "pipeline: failed to update status to deploying", "id", d.ID)
	}

	containerID, containerAddr, err := s.dockerSvc.RunContainer(ctx, imageTag, d.Subdomain)
	if err != nil {
		s.failPipeline(ctx, d, fmt.Errorf("run container: %w", err))
		return
	}

	d.ContainerID = &containerID
	d.ContainerAddr = &containerAddr

	if err := s.dockerSvc.WaitForHealthy(ctx, containerID, 10*time.Second); err != nil {
		logs, logErr := s.dockerSvc.GetContainerLogs(ctx, containerID)
		if logErr == nil && logs != "" {
			s.publishLogLines(ctx, d.ID, "runtime", "stderr", logs)
		}
		_ = s.dockerSvc.StopContainer(ctx, containerID)
		s.failPipeline(ctx, d, fmt.Errorf("health check failed: %w", err))
		return
	}

	if err := s.caddySvc.AddRoute(ctx, d.Subdomain, containerAddr); err != nil {
		_ = s.dockerSvc.StopContainer(ctx, containerID)
		s.failPipeline(ctx, d, fmt.Errorf("caddy route: %w", err))
		return
	}

	liveURL := "http://" + d.Subdomain + "." + s.cfg.Domain
	d.LiveURL = &liveURL
	d.Status = entities.StatusRunning

	if err := s.repo.Update(ctx, d); err != nil {
		logger.Error(err, "pipeline: failed to update status to running", "id", d.ID)
		return
	}

	logger.Info("deployment running", "id", d.ID, "url", liveURL, "container", containerID[:12])

	go s.streamRuntimeLogs(ctx, d.ID, containerID)
}

func (s *deploymentService) runFromImage(d *entities.Deployment) {
	ctx := context.Background()
	imageTag := *d.ImageTag
	logger.Info("pipeline (restart from image) started", "id", d.ID, "imageTag", imageTag)

	s.publishLogLines(ctx, d.ID, "deploy", "stdout", "Reusing image "+imageTag)

	d.Status = entities.StatusDeploying
	if err := s.repo.Update(ctx, d); err != nil {
		logger.Error(err, "pipeline: failed to update status to deploying", "id", d.ID)
	}

	s.publishLogLines(ctx, d.ID, "deploy", "stdout", "Starting container...")
	containerID, containerAddr, err := s.dockerSvc.RunContainer(ctx, imageTag, d.Subdomain)
	if err != nil {
		s.publishLogLines(ctx, d.ID, "deploy", "stderr", "Failed to start container: "+err.Error())
		s.failPipeline(ctx, d, fmt.Errorf("run container: %w", err))
		return
	}
	s.publishLogLines(ctx, d.ID, "deploy", "stdout", "Container started "+containerID[:12])

	d.ContainerID = &containerID
	d.ContainerAddr = &containerAddr

	s.publishLogLines(ctx, d.ID, "deploy", "stdout", "Waiting for health check...")
	if err := s.dockerSvc.WaitForHealthy(ctx, containerID, 10*time.Second); err != nil {
		logs, logErr := s.dockerSvc.GetContainerLogs(ctx, containerID)
		if logErr == nil && logs != "" {
			s.publishLogLines(ctx, d.ID, "runtime", "stderr", logs)
		}
		s.publishLogLines(ctx, d.ID, "deploy", "stderr", "Health check failed: "+err.Error())
		_ = s.dockerSvc.StopContainer(ctx, containerID)
		s.failPipeline(ctx, d, fmt.Errorf("health check failed: %w", err))
		return
	}
	s.publishLogLines(ctx, d.ID, "deploy", "stdout", "Health check passed")

	s.publishLogLines(ctx, d.ID, "deploy", "stdout", "Registering route for "+d.Subdomain+"...")
	if err := s.caddySvc.AddRoute(ctx, d.Subdomain, containerAddr); err != nil {
		s.publishLogLines(ctx, d.ID, "deploy", "stderr", "Failed to register route: "+err.Error())
		_ = s.dockerSvc.StopContainer(ctx, containerID)
		s.failPipeline(ctx, d, fmt.Errorf("caddy route: %w", err))
		return
	}

	liveURL := "http://" + d.Subdomain + "." + s.cfg.Domain
	d.LiveURL = &liveURL
	d.Status = entities.StatusRunning

	if err := s.repo.Update(ctx, d); err != nil {
		logger.Error(err, "pipeline: failed to update status to running", "id", d.ID)
		return
	}

	s.publishLogLines(ctx, d.ID, "deploy", "stdout", "Deployment live at "+liveURL)
	logger.Info("deployment restarted successfully", "id", d.ID, "url", liveURL, "container", containerID[:12])
	go s.streamRuntimeLogs(ctx, d.ID, containerID)
}

func (s *deploymentService) acquireSource(ctx context.Context, d *entities.Deployment, sourceDir string) error {
	if d.S3Key != nil && strings.TrimSpace(*d.S3Key) != "" {
		zipPath := filepath.Join(os.TempDir(), "brimble-"+d.ID+"-source.zip")
		defer os.Remove(zipPath)

		if err := s.downloadObjectToFile(ctx, *d.S3Key, zipPath); err != nil {
			return fmt.Errorf("download zip: %w", err)
		}
		if err := extractZip(zipPath, sourceDir); err != nil {
			return fmt.Errorf("extract zip: %w", err)
		}
		return nil
	}

	if d.GitURL == nil || strings.TrimSpace(*d.GitURL) == "" {
		return fmt.Errorf("source acquisition: git_url is required")
	}
	if err := s.builderSvc.Clone(ctx, *d.GitURL, sourceDir, d.ID); err != nil {
		return fmt.Errorf("clone: %w", err)
	}

	tarPath := filepath.Join(os.TempDir(), "brimble-"+d.ID+"-source.tar.gz")
	defer os.Remove(tarPath)
	if err := createTarGz(sourceDir, tarPath); err != nil {
		return fmt.Errorf("archive source: %w", err)
	}

	key := fmt.Sprintf("%s/source.tar.gz", d.ID)
	if err := s.uploadFileToS3(ctx, tarPath, key, "application/gzip"); err != nil {
		return fmt.Errorf("upload source archive: %w", err)
	}
	d.S3Key = &key
	if err := s.repo.Update(ctx, d); err != nil {
		return fmt.Errorf("persist s3 key: %w", err)
	}
	return nil
}

func (s *deploymentService) uploadFileToS3(ctx context.Context, path, key, contentType string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	if err := s.s3.Upload(ctx, key, f, contentType); err != nil {
		return err
	}
	return nil
}

func (s *deploymentService) downloadObjectToFile(ctx context.Context, key, destPath string) error {
	body, err := s.s3.Download(ctx, key)
	if err != nil {
		return err
	}
	defer body.Close()

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	_, copyErr := io.Copy(out, body)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return nil
}

func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	stripPrefix := detectZipRootPrefix(r.File)

	for _, f := range r.File {
		relativeName := strings.TrimPrefix(normalizeZipPath(f.Name), stripPrefix)
		relativeName = strings.TrimPrefix(relativeName, "/")
		if relativeName == "" {
			continue
		}

		targetPath := filepath.Join(destDir, filepath.FromSlash(relativeName))
		cleanDest := filepath.Clean(destDir) + string(os.PathSeparator)
		cleanTarget := filepath.Clean(targetPath)
		if cleanTarget != filepath.Clean(destDir) && !strings.HasPrefix(cleanTarget, cleanDest) {
			return fmt.Errorf("zip entry %q escapes destination", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(cleanTarget, 0o755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(cleanTarget), 0o755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		out, err := os.OpenFile(cleanTarget, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}

		_, copyErr := io.Copy(out, rc)
		closeErr := out.Close()
		rc.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}

	return nil
}

func detectZipRootPrefix(files []*zip.File) string {
	var common []string
	initialized := false

	for _, f := range files {
		name := strings.Trim(normalizeZipPath(f.Name), "/")
		if name == "" {
			continue
		}

		parts := strings.Split(name, "/")
		if !f.FileInfo().IsDir() {
			if len(parts) == 1 {
				return ""
			}
			parts = parts[:len(parts)-1]
		}
		if len(parts) == 0 {
			return ""
		}

		if !initialized {
			common = append([]string(nil), parts...)
			initialized = true
			continue
		}

		n := min(len(common), len(parts))
		i := 0
		for i < n && common[i] == parts[i] {
			i++
		}
		common = common[:i]
		if len(common) == 0 {
			return ""
		}
	}

	if len(common) == 0 {
		return ""
	}
	return strings.Join(common, "/") + "/"
}

func normalizeZipPath(name string) string {
	return strings.ReplaceAll(name, "\\", "/")
}

func createTarGz(sourceDir, outPath string) error {
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	gzw := gzip.NewWriter(out)
	defer gzw.Close()

	tw := tar.NewWriter(gzw)
	defer tw.Close()

	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == sourceDir {
			return nil
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relPath)
		if info.IsDir() {
			header.Name += "/"
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}

		_, copyErr := io.Copy(tw, f)
		closeErr := f.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func (s *deploymentService) failPipeline(ctx context.Context, d *entities.Deployment, err error) {
	logger.Error(err, "pipeline failed", "id", d.ID)
	d.Status = entities.StatusFailed
	msg := err.Error()
	d.ErrorMessage = &msg
	if updateErr := s.repo.Update(ctx, d); updateErr != nil {
		logger.Error(updateErr, "pipeline: failed to set status to failed", "id", d.ID)
	}
}

func (s *deploymentService) publishLogLines(ctx context.Context, deploymentID, phase, stream, content string) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if shouldSkipDeploymentLog(phase, line) {
			continue
		}
		log := &entities.DeploymentLog{
			DeploymentID: deploymentID,
			Stream:       stream,
			Phase:        phase,
			Content:      line,
		}
		if err := s.repo.InsertLog(ctx, log); err != nil {
			logger.Error(err, "failed to persist container log line", "deploymentID", deploymentID)
			continue
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
}

func (s *deploymentService) streamRuntimeLogs(ctx context.Context, deploymentID, containerID string) {
	logs, err := s.dockerSvc.StreamContainerLogs(ctx, containerID)
	if err != nil {
		logger.Error(err, "failed to start runtime log stream", "deploymentID", deploymentID, "containerID", containerID)
		return
	}
	defer logs.Close()

	stdout := &runtimeLogWriter{
		deploymentID: deploymentID,
		phase:        "runtime",
		stream:       "stdout",
		broker:       s.broker,
		repo:         s.repo,
		ctx:          ctx,
	}
	stderr := &runtimeLogWriter{
		deploymentID: deploymentID,
		phase:        "runtime",
		stream:       "stderr",
		broker:       s.broker,
		repo:         s.repo,
		ctx:          ctx,
	}

	_, err = stdcopy.StdCopy(stdout, stderr, logs)
	if err != nil && ctx.Err() == nil {
		logger.Error(err, "runtime log stream ended", "deploymentID", deploymentID, "containerID", containerID)
	}

	state, inspectErr := s.dockerSvc.InspectContainer(ctx, containerID)
	if inspectErr == nil && state != nil && !state.Running && state.ExitCode != 0 {
		logger.Info("container crashed after startup", "deploymentID", deploymentID, "exitCode", state.ExitCode)
		d, err := s.repo.GetByID(ctx, deploymentID)
		if err == nil && d != nil && d.Status == entities.StatusRunning {
			d.Status = entities.StatusFailed
			msg := fmt.Sprintf("container crashed with exit code %d", state.ExitCode)
			d.ErrorMessage = &msg
			if updateErr := s.repo.Update(ctx, d); updateErr != nil {
				logger.Error(updateErr, "failed to update deployment status after crash", "deploymentID", deploymentID)
			}
		}
	}
}

func (w *runtimeLogWriter) Write(p []byte) (n int, err error) {
	content := strings.TrimSpace(string(p))
	if content == "" {
		return len(p), nil
	}
	if shouldSkipDeploymentLog(w.phase, content) {
		return len(p), nil
	}
	log := &entities.DeploymentLog{
		DeploymentID: w.deploymentID,
		Stream:       w.stream,
		Phase:        w.phase,
		Content:      content,
	}
	if insertErr := w.repo.InsertLog(w.ctx, log); insertErr != nil {
		logger.Error(insertErr, "failed to persist runtime log", "deploymentID", w.deploymentID)
		return len(p), nil
	}
	w.broker.Publish(w.deploymentID, broker.LogLine{
		ID:           log.ID,
		DeploymentID: log.DeploymentID,
		Timestamp:    log.Timestamp.Format(time.RFC3339),
		Phase:        log.Phase,
		Stream:       log.Stream,
		Content:      log.Content,
	})
	return len(p), nil
}

func shouldSkipDeploymentLog(phase, content string) bool {
	if phase != "runtime" {
		return false
	}

	if strings.Contains(content, `"http.log.access`) {
		return true
	}

	if strings.Contains(content, `"logger":"http.log.error`) &&
		strings.Contains(content, `"/favicon.ico"`) &&
		(strings.Contains(content, `"status":404`) || strings.Contains(content, `HTTP 404`)) {
		return true
	}

	return false
}

func cloneDeployment(d *entities.Deployment) *entities.Deployment {
	if d == nil {
		return nil
	}

	copy := *d
	return &copy
}
