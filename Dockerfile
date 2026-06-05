# ---- Build Stage ----
FROM golang:1.25-alpine AS builder

ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown
ARG GOPROXY=https://proxy.golang.org,direct

WORKDIR /src
COPY go.mod go.sum ./
RUN GOPROXY=${GOPROXY} go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOPROXY=${GOPROXY} go build \
    -ldflags="-s -w \
      -X main.version=${VERSION} \
      -X main.commit=${COMMIT} \
      -X main.date=${DATE}" \
    -o /agent-usage .

# ---- Runtime Stage ----
FROM alpine:3.21

# Copy CA certs from builder (needed for HTTPS pricing sync)
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

RUN addgroup -g 1000 -S agentusage \
    && adduser -u 1000 -S -D -H -G agentusage agentusage \
    && mkdir -p /data /sessions/claude /sessions/codex /etc/agent-usage \
    && chown -R agentusage:agentusage /data /sessions /etc/agent-usage

COPY --from=builder /agent-usage /agent-usage
COPY --chown=agentusage:agentusage config.docker.yaml /etc/agent-usage/config.yaml

EXPOSE 9800

VOLUME ["/data"]

USER agentusage:agentusage

ENTRYPOINT ["/agent-usage"]
