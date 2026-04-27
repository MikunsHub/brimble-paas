FROM node:22-alpine AS frontend-builder

WORKDIR /app/frontend

COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

COPY frontend/ ./
RUN npm run build

FROM caddy:2

COPY Caddyfile /etc/caddy/Caddyfile
COPY --from=frontend-builder /app/frontend/dist /srv
