# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.25

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go test ./...

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o /out/yyb-go ./cmd/yyb-go

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone \
    && addgroup -S -g 10001 yyb \
    && adduser -S -D -H -u 10001 -G yyb yyb

WORKDIR /app

COPY --from=builder --chown=10001:10001 /out/yyb-go /app/yyb-go
COPY --chown=10001:10001 resource/templates /app/assets/templates
COPY --chown=10001:10001 resource/static /app/assets/static
COPY --chown=10001:10001 config/service.docker.json /app/config/service.json

RUN mkdir -p /app/data \
    && chown -R 10001:10001 /app/data /app/config /app/assets

USER 10001:10001

EXPOSE 8000

HEALTHCHECK --interval=15s --timeout=5s --start-period=30s --retries=5 \
    CMD wget -q -T 3 -O /dev/null http://127.0.0.1:8000/ready || exit 1

ENTRYPOINT ["/app/yyb-go"]
