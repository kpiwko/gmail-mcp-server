# Gmail MCP Server Container
#
# Build:
#   podman build -t localhost/gmail-mcp-server .
#
# Run (first time — no token yet):
#   podman volume create gmail-mcp-data
#   podman run -d --name gmail-mcp-server \
#     -p 6633:6633 \
#     -v gmail-mcp-data:/config \
#     -e GMAIL_CLIENT_ID=<your-client-id> \
#     -e GMAIL_CLIENT_SECRET=<your-client-secret> \
#     -e XDG_CONFIG_HOME=/config \
#     localhost/gmail-mcp-server
#   open http://localhost:6633/auth      ← authorize here, token is cached in the volume
#
# Subsequent runs:
#   Token is already in the volume — just start the container, /auth shows "authorized".
#
# Register with Claude Code (run once after authorizing):
#   claude mcp add --transport sse --scope user gmail http://localhost:6633/sse

# =============================================================================
# Stage 1: Builder
# =============================================================================
FROM golang:1.25 AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s" \
    -o gmail-mcp-server \
    ./cmd/gmail-mcp-server/

# =============================================================================
# Stage 2: Runtime
# =============================================================================
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

LABEL name="gmail-mcp-server" \
      summary="Gmail MCP Server" \
      description="MCP server providing Gmail access for AI agents via Device Authorization flow"

COPY --from=builder /build/gmail-mcp-server /usr/local/bin/gmail-mcp-server

# OAuth token and config files are stored in this volume.
# Set XDG_CONFIG_HOME=/config so the server uses /config/gmail-mcp-server/
# as its config directory on Linux (where token.json and credentials are stored).
VOLUME /config
ENV XDG_CONFIG_HOME=/config

EXPOSE 8080

CMD ["/usr/local/bin/gmail-mcp-server", "--http"]
