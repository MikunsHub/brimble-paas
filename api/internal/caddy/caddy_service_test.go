package caddy

import (
	"context"
	"encoding/json"
	"io"
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
		var route map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/config/apps/http/servers":
				_ = json.NewEncoder(w).Encode(map[string]any{"srv0": map[string]any{}})
			case "/config/apps/http/servers/srv0/routes/0":
				routePosted = true
				assert.Equal(t, http.MethodPut, r.Method)
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.NoError(t, json.Unmarshal(body, &route))
				w.WriteHeader(http.StatusOK)
			default:
				http.NotFound(w, r)
			}
		}))
		defer srv.Close()

		svc := NewCaddyService(srv.URL, "example.com")
		require.NoError(t, svc.AddRoute(context.Background(), "demo", "10.0.0.1:8000"))
		assert.True(t, routePosted)
		assert.Equal(t, "deployment-demo", route["@id"])
		match := route["match"].([]any)[0].(map[string]any)
		assert.Equal(t, []any{"demo.example.com"}, match["host"])
	})

	t.Run("non-200", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/config/apps/http/servers":
				_ = json.NewEncoder(w).Encode(map[string]any{"srv0": map[string]any{}})
			case "/config/apps/http/servers/srv0/routes/0":
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
