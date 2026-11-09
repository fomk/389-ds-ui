# syntax=docker/dockerfile:1
# Build context: repository root.
#   docker build -f deploy/ui/Dockerfile .                   → sidecar image (default)
#   docker build -f deploy/ui/Dockerfile --target static .   → remote-only image (distroless)

FROM node:22-slim AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-fund --no-audit
COPY web/ ./
COPY api/ /src/api/
RUN mkdir -p /src/internal/webui/dist && npm run build

FROM golang:1.26 AS go
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY --from=web /src/internal/webui/dist/ internal/webui/dist/
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/ds-ui ./cmd/ds-ui

# Remote mode: LDAP only, no files, no certutil.
FROM gcr.io/distroless/static-debian12:nonroot AS static
COPY --from=go /out/ds-ui /usr/local/bin/ds-ui
ENV DSUI_MODE=remote
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s CMD ["/usr/local/bin/ds-ui", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/ds-ui"]

# Sidecar mode: shares /data with the ds container. Runs as dirsrv (uid 389),
# which 389-ds maps to cn=Directory Manager on LDAPI autobind.
FROM debian:trixie-slim AS sidecar
ARG DS_UID=389
ARG DS_GID=389
RUN groupadd --system --gid "${DS_GID}" dirsrv \
 && useradd --system --uid "${DS_UID}" --gid dirsrv --no-create-home --home-dir /nonexistent \
      --shell /usr/sbin/nologin dirsrv \
 && apt-get update \
 && apt-get install -y --no-install-recommends libnss3-tools ca-certificates \
 && rm -rf /var/lib/apt/lists/*
COPY --from=go /out/ds-ui /usr/local/bin/ds-ui
USER dirsrv
ENV DSUI_MODE=sidecar
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s CMD ["/usr/local/bin/ds-ui", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/ds-ui"]
