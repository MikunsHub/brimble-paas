package caddy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type CaddyService struct {
	adminURL   string
	domain     string
	httpClient *http.Client
}

func NewCaddyService(adminURL, domain string) *CaddyService {
	return &CaddyService{
		adminURL:   adminURL,
		domain:     domain,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *CaddyService) AddRoute(ctx context.Context, subdomain, upstreamAddr string) error {
	serverName, err := s.discoverServerName(ctx)
	if err != nil {
		return err
	}

	route := map[string]any{
		"@id": "deployment-" + subdomain,
		"match": []map[string]any{
			{"host": []string{subdomain + "." + s.domain}},
		},
		"handle": []map[string]any{
			{
				"handler": "reverse_proxy",
				"upstreams": []map[string]any{
					{"dial": upstreamAddr},
				},
			},
		},
	}

	body, err := json.Marshal(route)
	if err != nil {
		return fmt.Errorf("failed to marshal caddy route: %w", err)
	}

	// Insert deployment routes ahead of the platform catch-all so
	// subdomain traffic reaches the deployed app instead of /srv.
	url := fmt.Sprintf("%s/config/apps/http/servers/%s/routes/0", s.adminURL, serverName)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("caddy route add failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("caddy returned %d when adding route for %s", resp.StatusCode, subdomain)
	}
	return nil
}

func (s *CaddyService) RemoveRoute(ctx context.Context, subdomain string) error {
	url := fmt.Sprintf("%s/id/deployment-%s", s.adminURL, subdomain)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("caddy route remove failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("caddy returned %d when removing route for %s", resp.StatusCode, subdomain)
	}
	return nil
}

func (s *CaddyService) discoverServerName(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.adminURL+"/config/apps/http/servers", nil)
	if err != nil {
		return "", err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("caddy admin unreachable: %w", err)
	}
	defer resp.Body.Close()

	var servers map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&servers); err != nil {
		return "", fmt.Errorf("failed to decode caddy servers: %w", err)
	}
	for name := range servers {
		return name, nil
	}
	return "", fmt.Errorf("no HTTP servers found in Caddy config")
}
