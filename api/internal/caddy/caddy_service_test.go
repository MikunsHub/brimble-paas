package caddy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCaddyService(t *testing.T) {
	t.Parallel()

	svc := NewCaddyService("http://localhost:2019", "example.com")
	require.NotNil(t, svc)
	assert.Equal(t, "http://localhost:2019", svc.adminURL)
	assert.Equal(t, "example.com", svc.domain)
	require.NotNil(t, svc.httpClient)
}

func TestCaddyService_DiscoverServerName(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/config/apps/http/servers", r.URL.Path)
			_ = json.NewEncoder(w).Encode(map[string]any{"srv0": map[string]any{}})
		}))
		defer srv.Close()

		svc := NewCaddyService(srv.URL, "example.com")
		name, err := svc.discoverServerName(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "srv0", name)
	})

	t.Run("decode error", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("{"))
		}))
		defer srv.Close()

		svc := NewCaddyService(srv.URL, "example.com")
		_, err := svc.discoverServerName(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode caddy servers")
	})

	t.Run("no servers", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{})
		}))
		defer srv.Close()

		svc := NewCaddyService(srv.URL, "example.com")
		_, err := svc.discoverServerName(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no HTTP servers found")
	})
}

func TestCaddyService_AddRoute(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		var routePosted bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/config/apps/http/servers":
				_ = json.NewEncoder(w).Encode(map[string]any{"srv0": map[string]any{}})
			case "/config/apps/http/servers/srv0/routes":
				routePosted = true
				assert.Equal(t, http.MethodPost, r.Method)
				w.WriteHeader(http.StatusOK)
			default:
				http.NotFound(w, r)
			}
		}))
		defer srv.Close()

		svc := NewCaddyService(srv.URL, "example.com")
		require.NoError(t, svc.AddRoute(context.Background(), "demo", "10.0.0.1:8000"))
		assert.True(t, routePosted)
	})

	t.Run("non-200", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/config/apps/http/servers":
				_ = json.NewEncoder(w).Encode(map[string]any{"srv0": map[string]any{}})
			case "/config/apps/http/servers/srv0/routes":
				w.WriteHeader(http.StatusBadGateway)
			}
		}))
		defer srv.Close()

		svc := NewCaddyService(srv.URL, "example.com")
		err := svc.AddRoute(context.Background(), "demo", "10.0.0.1:8000")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "caddy returned 502")
	})
}

func TestCaddyService_RemoveRoute(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/id/deployment-demo", r.URL.Path)
			assert.Equal(t, http.MethodDelete, r.Method)
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		svc := NewCaddyService(srv.URL, "example.com")
		require.NoError(t, svc.RemoveRoute(context.Background(), "demo"))
	})

	t.Run("non-200", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		svc := NewCaddyService(srv.URL, "example.com")
		err := svc.RemoveRoute(context.Background(), "demo")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "caddy returned 500")
	})
}
