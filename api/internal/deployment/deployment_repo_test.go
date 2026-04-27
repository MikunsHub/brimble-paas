package deployment

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/brimble/paas/entities"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPostgresRepo_Create(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	repo := NewDeploymentRepo(db)

	d := &entities.Deployment{
		ID:        "dep-create",
		Subdomain: "demo-app",
		Status:    entities.StatusPending,
	}

	require.NoError(t, repo.Create(context.Background(), d))
	assert.False(t, d.CreatedAt.IsZero())
	assert.False(t, d.UpdatedAt.IsZero())

	var persisted entities.Deployment
	require.NoError(t, db.First(&persisted, "id = ?", d.ID).Error)
	assert.Equal(t, d.Subdomain, persisted.Subdomain)
	assert.Equal(t, d.Status, persisted.Status)
}

func TestPostgresRepo_GetByID(t *testing.T) {
	t.Parallel()

	t.Run("found returns record", func(t *testing.T) {
		t.Parallel()
		db := setupTestDB(t)
		repo := NewDeploymentRepo(db)
		seedDeployment(t, db, &entities.Deployment{ID: "dep-found", Subdomain: "found", Status: entities.StatusRunning})

		got, err := repo.GetByID(context.Background(), "dep-found")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "found", got.Subdomain)
	})

	t.Run("not found returns nil", func(t *testing.T) {
		t.Parallel()
		db := setupTestDB(t)
		repo := NewDeploymentRepo(db)

		got, err := repo.GetByID(context.Background(), "missing")
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("error propagates", func(t *testing.T) {
		t.Parallel()
		db := setupTestDB(t)
		repo := NewDeploymentRepo(db)
		closeTestDB(t, db)

		got, err := repo.GetByID(context.Background(), "dep")
		require.Error(t, err)
		assert.Nil(t, got)
	})
}

func TestPostgresRepo_List(t *testing.T) {
	t.Parallel()

	t.Run("returns ordered list", func(t *testing.T) {
		t.Parallel()
		db := setupTestDB(t)
		repo := NewDeploymentRepo(db)

		now := time.Now().UTC()
		seedDeployment(t, db, &entities.Deployment{ID: "older", Subdomain: "older", Status: entities.StatusPending, CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour)})
		seedDeployment(t, db, &entities.Deployment{ID: "newer", Subdomain: "newer", Status: entities.StatusRunning, CreatedAt: now, UpdatedAt: now})

		list, err := repo.List(context.Background())
		require.NoError(t, err)
		require.Len(t, list, 2)
		assert.Equal(t, "newer", list[0].ID)
		assert.Equal(t, "older", list[1].ID)
	})

	t.Run("empty table returns empty slice", func(t *testing.T) {
		t.Parallel()
		db := setupTestDB(t)
		repo := NewDeploymentRepo(db)

		list, err := repo.List(context.Background())
		require.NoError(t, err)
		assert.Empty(t, list)
	})
}

func TestPostgresRepo_Update(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	repo := NewDeploymentRepo(db)
	createdAt := time.Now().UTC().Add(-time.Hour)
	deployment := &entities.Deployment{ID: "dep-update", Subdomain: "demo", Status: entities.StatusPending, CreatedAt: createdAt, UpdatedAt: createdAt}
	seedDeployment(t, db, deployment)

	got, err := repo.GetByID(context.Background(), deployment.ID)
	require.NoError(t, err)
	require.NotNil(t, got)

	got.Status = entities.StatusRunning
	got.LiveURL = stringPtr("http://demo.example.com")
	beforeUpdate := got.UpdatedAt

	require.NoError(t, repo.Update(context.Background(), got))

	updated, err := repo.GetByID(context.Background(), deployment.ID)
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, entities.StatusRunning, updated.Status)
	require.NotNil(t, updated.LiveURL)
	assert.Equal(t, "http://demo.example.com", *updated.LiveURL)
	assert.True(t, updated.UpdatedAt.After(beforeUpdate) || updated.UpdatedAt.Equal(beforeUpdate))
}

func TestPostgresRepo_Delete(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	repo := NewDeploymentRepo(db)
	seedDeployment(t, db, &entities.Deployment{ID: "dep-delete", Subdomain: "demo", Status: entities.StatusPending})

	require.NoError(t, repo.Delete(context.Background(), "dep-delete"))

	got, err := repo.GetByID(context.Background(), "dep-delete")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestPostgresRepo_InsertLog(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	repo := NewDeploymentRepo(db)
	seedDeployment(t, db, &entities.Deployment{ID: "dep-log", Subdomain: "demo", Status: entities.StatusRunning})

	log := &entities.DeploymentLog{
		ID:           "log-1",
		DeploymentID: "dep-log",
		Stream:       "stdout",
		Phase:        "build",
		Content:      "ready",
	}

	require.NoError(t, repo.InsertLog(context.Background(), log))
	assert.False(t, log.Timestamp.IsZero())

	var persisted entities.DeploymentLog
	require.NoError(t, db.First(&persisted, "id = ?", "log-1").Error)
	assert.Equal(t, "dep-log", persisted.DeploymentID)
}

func TestPostgresRepo_GetLogs(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	repo := NewDeploymentRepo(db)
	seedDeployment(t, db, &entities.Deployment{ID: "dep-logs", Subdomain: "demo", Status: entities.StatusRunning})

	now := time.Now().UTC()
	seedLog(t, db, &entities.DeploymentLog{ID: "log-1", DeploymentID: "dep-logs", Timestamp: now.Add(-2 * time.Minute), Stream: "stdout", Phase: "build", Content: "one"})
	seedLog(t, db, &entities.DeploymentLog{ID: "log-2", DeploymentID: "dep-logs", Timestamp: now.Add(-1 * time.Minute), Stream: "stdout", Phase: "build", Content: "two"})
	seedLog(t, db, &entities.DeploymentLog{ID: "log-3", DeploymentID: "dep-logs", Timestamp: now, Stream: "stdout", Phase: "runtime", Content: "three"})

	logs, err := repo.GetLogs(context.Background(), "dep-logs", 1)
	require.NoError(t, err)
	require.Len(t, logs, 2)
	assert.Equal(t, "log-2", logs[0].ID)
	assert.Equal(t, "log-3", logs[1].ID)
}

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`
		CREATE TABLE deployments (
			id TEXT PRIMARY KEY,
			git_url TEXT,
			s3_key TEXT,
			subdomain TEXT NOT NULL UNIQUE,
			status TEXT NOT NULL DEFAULT 'pending',
			image_tag TEXT,
			container_id TEXT,
			container_addr TEXT,
			live_url TEXT,
			detected_lang TEXT,
			start_cmd TEXT,
			error_message TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE deployment_logs (
			id TEXT PRIMARY KEY,
			deployment_id TEXT NOT NULL,
			timestamp DATETIME,
			stream TEXT NOT NULL,
			phase TEXT NOT NULL,
			content TEXT NOT NULL
		)
	`).Error)
	return db
}

func closeTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
}

func seedDeployment(t *testing.T, db *gorm.DB, d *entities.Deployment) {
	t.Helper()
	require.NoError(t, db.Create(d).Error)
}

func seedLog(t *testing.T, db *gorm.DB, l *entities.DeploymentLog) {
	t.Helper()
	require.NoError(t, db.Create(l).Error)
}

func stringPtr(value string) *string {
	return &value
}
