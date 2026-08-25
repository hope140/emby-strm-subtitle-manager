# syntax=docker/dockerfile:1

FROM golang:1.26.7-alpine3.24 AS build

ARG BUILD_VERSION=dev
ARG BUILD_COMMIT=unknown
ARG BUILD_TIME=unknown

WORKDIR /src
ENV CGO_ENABLED=0

# Keep dependency resolution in its own layer and build only the server
# binary. The build context is further restricted by .dockerignore.
COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
RUN go build -trimpath -buildvcs=false -ldflags="-s -w -X github.com/hope140/subbridge/internal/version.Version=${BUILD_VERSION} -X github.com/hope140/subbridge/internal/version.Commit=${BUILD_COMMIT} -X github.com/hope140/subbridge/internal/version.BuildTime=${BUILD_TIME}" -o /out/subbridge ./cmd/server

FROM alpine:3.24.1

ARG BUILD_VERSION=dev
ARG BUILD_COMMIT=unknown
ARG BUILD_TIME=unknown
ARG BUILD_SOURCE=https://github.com/hope140/subbridge

LABEL org.opencontainers.image.version="${BUILD_VERSION}" \
      org.opencontainers.image.revision="${BUILD_COMMIT}" \
      org.opencontainers.image.created="${BUILD_TIME}" \
      org.opencontainers.image.source="${BUILD_SOURCE}"

RUN apk add --no-cache ca-certificates \
    && addgroup -S -g 10001 app \
    && adduser -S -D -H -u 10001 -G app app

COPY --from=build --chown=app:app /out/subbridge /usr/local/bin/subbridge

USER 10001:10001
EXPOSE 8080

# BusyBox wget is present in the final Alpine image and makes the existing
# read-only liveness endpoint usable as an image-level process healthcheck.
# Compose performs the stricter readiness check against /readyz.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["wget", "-q", "-O", "/dev/null", "http://127.0.0.1:8080/livez"]

ENTRYPOINT ["/usr/local/bin/subbridge"]
