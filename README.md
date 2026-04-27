# brimble-paas

One-page deployment pipeline for containerized apps built with Vite + TanStack, a Go API, Railpack builds, Docker runtime, and Caddy ingress.

## Run

Prerequisites:

- Docker with Compose enabled
- Internet access for image pulls and Railpack installation inside the API image

Start everything:

```bash
docker compose up --build
```

UI:

- `http://localhost`

API health:

- `http://localhost/api/health`

Deployment URLs:

- Each deployment is exposed on `http://<deployment-subdomain>.localhost`
- `*.localhost` resolves to loopback on modern systems and browsers, so no `dnsmasq` or `/etc/hosts` wildcard setup is required

## What Starts

- `caddy`: single ingress, serves the built frontend and proxies `/api/*`
- `api`: Go service, runs `goose` migrations on startup, builds apps with Railpack, starts containers with Docker, and registers routes in Caddy
- `postgres`: deployment state and persisted logs
- `localstack`: S3-compatible storage for uploaded source archives
- `buildkit`: backend build engine used by Railpack

## Notes

- Migrations are managed with `goose` and run automatically from the API container entrypoint
- The frontend is built into the Caddy image, so there is no separate frontend runtime container
- Logs are persisted in Postgres and streamed live to the UI over SSE
