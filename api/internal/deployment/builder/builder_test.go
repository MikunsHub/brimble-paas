package builder

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/brimble/paas/entities"
	"github.com/brimble/paas/pkg/broker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeMode(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "prod", NormalizeMode("prod"))
	assert.Equal(t, "prod", NormalizeMode("production"))
	assert.Equal(t, "dev", NormalizeMode("dev"))
	assert.Equal(t, "dev", NormalizeMode("anything-else"))
}

func TestService_IsDevMode(t *testing.T) {
	t.Parallel()

	assert.True(t, NewService("dev", nil, nil).IsDevMode())
	assert.False(t, NewService("prod", nil, nil).IsDevMode())
}

func TestService_IsProdMode(t *testing.T) {
	t.Parallel()

	assert.True(t, NewService("prod", nil, nil).IsProdMode())
	assert.False(t, NewService("dev", nil, nil).IsProdMode())
}

func TestService_String(t *testing.T) {
	t.Parallel()

	assert.Contains(t, NewService("prod", nil, nil).String(), "production")
	assert.Contains(t, NewService("dev", nil, nil).String(), "development")
}

func TestService_Clone(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		sink := &fakeSink{}
		pub := &fakePublisher{}
		runner := &fakeRunner{
			commands: []*fakeCommand{
				newFakeCommand("cloned\n", "", nil),
			},
		}
		svc := newTestService("dev", pub, sink, runner)

		err := svc.Clone(context.Background(), "https://github.com/brimble/app.git", "/tmp/repo", "dep-1")
		require.NoError(t, err)
		require.Len(t, runner.calls, 1)
		assert.Equal(t, "git", runner.calls[0].name)
		assert.Equal(t, []string{"clone", "--depth", "1", "https://github.com/brimble/app.git", "/tmp/repo"}, runner.calls[0].args)
		require.Len(t, sink.logs, 1)
		assert.Equal(t, "clone", sink.logs[0].Phase)
		assert.Equal(t, "cloned", sink.logs[0].Content)
		require.Len(t, pub.lines, 1)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()
		svc := newTestService("dev", &fakePublisher{}, &fakeSink{}, &fakeRunner{
			commands: []*fakeCommand{newFakeCommand("", "", errors.New("boom"))},
		})

		err := svc.Clone(context.Background(), "git-url", "/tmp/repo", "dep-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "git clone failed")
	})
}

func TestService_BuildDev(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		sink := &fakeSink{}
		runner := &fakeRunner{
			commands: []*fakeCommand{
				newFakeCommand("building\n", "warning\n", nil),
			},
		}
		svc := newTestService("dev", &fakePublisher{}, sink, runner)

		info, err := svc.Build(context.Background(), "/src", "demo:latest", "dep-1")
		require.NoError(t, err)
		require.NotNil(t, info)
		require.Len(t, runner.calls, 1)
		assert.Equal(t, "railpack", runner.calls[0].name)
		assert.Equal(t, []string{"build", "/src", "--name", "demo:latest"}, runner.calls[0].args)
		require.Len(t, sink.logs, 2)
		streams := []string{sink.logs[0].Stream, sink.logs[1].Stream}
		assert.ElementsMatch(t, []string{"stdout", "stderr"}, streams)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()
		svc := newTestService("dev", &fakePublisher{}, &fakeSink{}, &fakeRunner{
			commands: []*fakeCommand{newFakeCommand("", "", errors.New("build failed"))},
		})

		info, err := svc.Build(context.Background(), "/src", "demo:latest", "dep-1")
		require.Error(t, err)
		assert.Nil(t, info)
		assert.Contains(t, err.Error(), "railpack build failed")
	})
}

func TestService_BuildProd(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		infoPath := filepath.Join(dir, "railpack-info.json")
		require.NoError(t, os.WriteFile(infoPath, []byte(`{"provider":{"name":"go"},"startCmd":"./app"}`), 0o644))

		runner := &fakeRunner{
			commands: []*fakeCommand{
				newFakeCommand("", "", nil),
				newFakeCommand("docker build\n", "", nil),
			},
		}
		svc := newTestService("prod", &fakePublisher{}, &fakeSink{}, runner)

		info, err := svc.Build(context.Background(), dir, "demo:latest", "dep-1")
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "go", info.DetectedLang)
		assert.Equal(t, "./app", info.StartCmd)
		require.Len(t, runner.calls, 2)
		assert.Equal(t, "railpack", runner.calls[0].name)
		assert.Equal(t, "docker", runner.calls[1].name)
		assert.Contains(t, strings.Join(runner.calls[1].args, " "), "buildx build")
	})

	t.Run("prepare error", func(t *testing.T) {
		t.Parallel()
		svc := newTestService("prod", &fakePublisher{}, &fakeSink{}, &fakeRunner{
			commands: []*fakeCommand{newFakeCommand("", "", errors.New("prepare failed"))},
		})

		info, err := svc.Build(context.Background(), t.TempDir(), "demo:latest", "dep-1")
		require.Error(t, err)
		assert.Nil(t, info)
		assert.Contains(t, err.Error(), "railpack prepare failed")
	})

	t.Run("docker build error", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "railpack-info.json"), []byte(`{"provider":{"name":"go"}}`), 0o644))
		svc := newTestService("prod", &fakePublisher{}, &fakeSink{}, &fakeRunner{
			commands: []*fakeCommand{
				newFakeCommand("", "", nil),
				newFakeCommand("", "", errors.New("docker failed")),
			},
		})

		info, err := svc.Build(context.Background(), dir, "demo:latest", "dep-1")
		require.Error(t, err)
		assert.Nil(t, info)
		assert.Contains(t, err.Error(), "docker buildx build failed")
	})
}

func TestService_StreamCommandErrors(t *testing.T) {
	t.Parallel()

	t.Run("stdout pipe error", func(t *testing.T) {
		t.Parallel()
		svc := newTestService("dev", &fakePublisher{}, &fakeSink{}, &fakeRunner{})
		err := svc.streamCommand(context.Background(), &fakeCommand{stdoutErr: errors.New("stdout")}, "dep-1", "build")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create stdout pipe")
	})

	t.Run("stderr pipe error", func(t *testing.T) {
		t.Parallel()
		svc := newTestService("dev", &fakePublisher{}, &fakeSink{}, &fakeRunner{})
		err := svc.streamCommand(context.Background(), &fakeCommand{stdout: io.NopCloser(strings.NewReader("")), stderrErr: errors.New("stderr")}, "dep-1", "build")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create stderr pipe")
	})

	t.Run("start error", func(t *testing.T) {
		t.Parallel()
		svc := newTestService("dev", &fakePublisher{}, &fakeSink{}, &fakeRunner{})
		err := svc.streamCommand(context.Background(), &fakeCommand{
			stdout:   io.NopCloser(strings.NewReader("")),
			stderr:   io.NopCloser(strings.NewReader("")),
			startErr: errors.New("start"),
		}, "dep-1", "build")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to start command")
	})
}

func TestParseBuildInfo(t *testing.T) {
	t.Parallel()

	t.Run("provider and start cmd", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "railpack-info.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"provider":{"name":"go"},"startCmd":"./app"}`), 0o644))

		info, err := parseBuildInfo(path)
		require.NoError(t, err)
		assert.Equal(t, "go", info.DetectedLang)
		assert.Equal(t, "./app", info.StartCmd)
	})

	t.Run("fallback name field", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "railpack-info.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"name":"node"}`), 0o644))

		info, err := parseBuildInfo(path)
		require.NoError(t, err)
		assert.Equal(t, "node", info.DetectedLang)
	})

	t.Run("invalid json", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "railpack-info.json")
		require.NoError(t, os.WriteFile(path, []byte(`{`), 0o644))

		info, err := parseBuildInfo(path)
		require.Error(t, err)
		assert.Nil(t, info)
	})
}

func TestCleanSourceDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	planPath := filepath.Join(dir, "railpack-plan.json")
	infoPath := filepath.Join(dir, "railpack-info.json")
	keepPath := filepath.Join(dir, "main.go")

	require.NoError(t, os.WriteFile(planPath, []byte("plan"), 0o644))
	require.NoError(t, os.WriteFile(infoPath, []byte("info"), 0o644))
	require.NoError(t, os.WriteFile(keepPath, []byte("package main"), 0o644))

	CleanSourceDir(dir)

	_, planErr := os.Stat(planPath)
	assert.ErrorIs(t, planErr, os.ErrNotExist)
	_, infoErr := os.Stat(infoPath)
	assert.ErrorIs(t, infoErr, os.ErrNotExist)
	_, keepErr := os.Stat(keepPath)
	assert.NoError(t, keepErr)
}

func TestService_Validate(t *testing.T) {
	t.Parallel()

	t.Run("success in dev mode", func(t *testing.T) {
		t.Parallel()
		svc := newTestService("dev", &fakePublisher{}, &fakeSink{}, &fakeRunner{
			lookPathResults: map[string]error{},
		})
		require.NoError(t, svc.Validate())
	})

	t.Run("git missing", func(t *testing.T) {
		t.Parallel()
		svc := newTestService("dev", &fakePublisher{}, &fakeSink{}, &fakeRunner{
			lookPathResults: map[string]error{"git": errors.New("missing")},
		})
		err := svc.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "git not found")
	})

	t.Run("docker required in prod", func(t *testing.T) {
		t.Parallel()
		svc := newTestService("prod", &fakePublisher{}, &fakeSink{}, &fakeRunner{
			lookPathResults: map[string]error{"docker": errors.New("missing")},
		})
		err := svc.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "docker CLI not found")
	})
}

type fakeSink struct {
	logs []*entities.DeploymentLog
}

func (s *fakeSink) InsertLog(_ context.Context, l *entities.DeploymentLog) error {
	l.ID = "log-id"
	if l.Timestamp.IsZero() {
		l.Timestamp = time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	}
	s.logs = append(s.logs, l)
	return nil
}

type fakePublisher struct {
	lines []broker.LogLine
}

func (p *fakePublisher) Publish(_ string, line broker.LogLine) error {
	p.lines = append(p.lines, line)
	return nil
}

func (p *fakePublisher) Subscribe(string) (<-chan broker.LogLine, func(), error) {
	ch := make(chan broker.LogLine)
	return ch, func() { close(ch) }, nil
}

type fakeRunner struct {
	commands        []*fakeCommand
	calls           []runnerCall
	lookPathResults map[string]error
}

type runnerCall struct {
	name string
	args []string
}

func (r *fakeRunner) CommandContext(_ context.Context, name string, args ...string) command {
	r.calls = append(r.calls, runnerCall{name: name, args: append([]string(nil), args...)})
	if len(r.commands) == 0 {
		return newFakeCommand("", "", nil)
	}
	cmd := r.commands[0]
	r.commands = r.commands[1:]
	return cmd
}

func (r *fakeRunner) LookPath(file string) (string, error) {
	if err := r.lookPathResults[file]; err != nil {
		return "", err
	}
	return "/usr/bin/" + file, nil
}

type fakeCommand struct {
	stdout    io.ReadCloser
	stderr    io.ReadCloser
	stdoutErr error
	stderrErr error
	startErr  error
	waitErr   error
}

func newFakeCommand(stdout, stderr string, waitErr error) *fakeCommand {
	return &fakeCommand{
		stdout:  io.NopCloser(strings.NewReader(stdout)),
		stderr:  io.NopCloser(strings.NewReader(stderr)),
		waitErr: waitErr,
	}
}

func (c *fakeCommand) StdoutPipe() (io.ReadCloser, error) {
	if c.stdoutErr != nil {
		return nil, c.stdoutErr
	}
	if c.stdout == nil {
		return io.NopCloser(strings.NewReader("")), nil
	}
	return c.stdout, nil
}

func (c *fakeCommand) StderrPipe() (io.ReadCloser, error) {
	if c.stderrErr != nil {
		return nil, c.stderrErr
	}
	if c.stderr == nil {
		return io.NopCloser(strings.NewReader("")), nil
	}
	return c.stderr, nil
}

func (c *fakeCommand) Start() error {
	return c.startErr
}

func (c *fakeCommand) Wait() error {
	return c.waitErr
}

func newTestService(mode string, pub broker.LogPublisher, sink LogSink, runner commandRunner) *Service {
	svc := NewService(mode, pub, sink)
	svc.runner = runner
	return svc
}
