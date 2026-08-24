# syntax=docker/dockerfile:1

FROM golang:1.26.7-alpine3.22 AS build

WORKDIR /src
ENV CGO_ENABLED=0

# Keep dependency resolution in its own layer and build only the server
# binary. The build context is further restricted by .dockerignore.
COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
RUN go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/emby-strm-subtitle-manager ./cmd/server

FROM alpine:3.22

RUN apk add --no-cache ca-certificates \
    && addgroup -S -g 10001 app \
    && adduser -S -D -H -u 10001 -G app app

COPY --from=build --chown=app:app /out/emby-strm-subtitle-manager /usr/local/bin/emby-strm-subtitle-manager

USER 10001:10001
EXPOSE 8080

# BusyBox wget is present in the final Alpine image and makes the existing
# read-only liveness endpoint usable as an image-level process healthcheck.
# Compose performs the stricter readiness check against /readyz.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["wget", "-q", "-O", "/dev/null", "http://127.0.0.1:8080/livez"]

ENTRYPOINT ["/usr/local/bin/emby-strm-subtitle-manager"]
