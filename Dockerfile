# ---- Build Stage ----
FROM golang:1.25.11-alpine AS builder

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
    -o /agent-ledger .

# ---- Runtime Stage ----
FROM alpine:3.21

# Copy CA certs from builder (needed for HTTPS pricing sync)
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

RUN addgroup -g 1000 -S agentledger \
    && adduser -u 1000 -S -D -H -G agentledger agentledger \
    && mkdir -p /data /sessions/claude /sessions/codex /sessions/opencode /etc/agent-ledger \
    && chown -R agentledger:agentledger /data /sessions /etc/agent-ledger

COPY --from=builder /agent-ledger /agent-ledger
COPY --chown=agentledger:agentledger config.docker.yaml /etc/agent-ledger/config.yaml

EXPOSE 9800

VOLUME ["/data"]

USER agentledger:agentledger

ENTRYPOINT ["/agent-ledger"]
