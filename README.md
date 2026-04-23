# Gmail MCP Server

An MCP server that lets AI agents search Gmail threads, understand your email writing style, and create draft emails.

## Prerequisites

- Go 1.25+
- A Google Cloud project with the Gmail API enabled (see [Google Setup](#1-google-setup))
- Optional: an OpenAI API key for auto-generating a personal writing style guide

## Quick start

```bash
# 1. Build
make build

# 2. Run (stdio mode — your MCP client manages the process)
GMAIL_CLIENT_ID=... GMAIL_CLIENT_SECRET=... ./bin/gmail-mcp-server

# 3. Or as a persistent HTTP server on port 6633
GMAIL_CLIENT_ID=... GMAIL_CLIENT_SECRET=... ./bin/gmail-mcp-server --http

# Show all flags
./gmail-mcp-server help
./gmail-mcp-server --help
```

On first run a browser window opens for Gmail OAuth. Approve it once — the token is cached locally for subsequent runs.

---

## 1. Google Setup

### Create a project and enable the Gmail API

1. Open [Google Cloud Console](https://console.cloud.google.com/)
2. Create a new project (or reuse an existing one)
3. Go to **APIs & Services → Library**, search for **Gmail API**, click **Enable**

### Create OAuth credentials

1. Go to **APIs & Services → Credentials → Create Credentials → OAuth Client ID**
2. If prompted, configure the OAuth consent screen:
   - User type: **External**
   - Fill in App name, support email, developer email
   - Add your own email under **Test users**
3. Application type: **Desktop application**
4. Click **Create** — copy the **Client ID** and **Client Secret**

### OAuth scopes requested

| Scope | Why |
|---|---|
| `gmail.readonly` | Search and read emails, download attachments |
| `gmail.compose` | Create and update drafts |

The server never sends emails or deletes anything.

---

## 2. Installation

### Container (recommended — no Go toolchain needed)

Build the image once from the project root:

```bash
podman build -t localhost/gmail-mcp-server .
```

Create a named volume for the OAuth token (persists across container restarts):

```bash
podman volume create gmail-mcp-data
```

Run the server:

```bash
podman run -d --name gmail-mcp-server \
  -p 6633:6633 \
  -v gmail-mcp-data:/config \
  -e GMAIL_CLIENT_ID=<your-client-id> \
  -e GMAIL_CLIENT_SECRET=<your-client-secret> \
  -e XDG_CONFIG_HOME=/config \
  localhost/gmail-mcp-server
```

**First run — authorize Gmail access:**

```bash
open http://localhost:6633/auth
```

The page shows a Google link and a short verification code. Click the link,
sign in, enter the code if prompted, and the page updates to show
"Authorized". The token is saved in the volume — no restart needed.

**Register with Claude Code** (once, after authorizing):

```bash
claude mcp add --transport http --scope user gmail http://localhost:6633/mcp
```

**Subsequent runs** just start the container — the cached token is loaded from
the volume automatically. Visit `/auth` at any time to check status or
re-authorize if the token ever expires.

### From source

```bash
git clone https://github.com/your-org/gmail-mcp-server
cd gmail-mcp-server
make build              # produces bin/gmail-mcp-server
make install            # copies to $GOPATH/bin/gmail-mcp-server
```

### Pre-built binaries

Download from the [Releases](../../releases) page. Binaries are available for:
- Linux amd64 / arm64
- macOS amd64 / arm64
- Windows amd64

---

## 3. Configuration

### Credentials — two options

**Option A: `credentials.json` file (recommended for local use)**

Download the credentials file directly from Google Cloud Console:

1. Go to **APIs & Services → Credentials**
2. Click the pencil icon next to your OAuth client → **Download JSON**
3. Save the file as `credentials.json` in the config directory

The file looks like this:
```json
{
  "installed": {
    "client_id": "123….apps.googleusercontent.com",
    "project_id": "my-project",
    "auth_uri": "https://accounts.google.com/o/oauth2/auth",
    "token_uri": "https://oauth2.googleapis.com/token",
    "client_secret": "GOCSPX-…",
    "redirect_uris": ["http://localhost"]
  }
}
```

Run `./gmail-mcp-server help` to see the exact config directory path on your machine.

**Option B: environment variables (recommended for MCP client configs / containers)**

| Variable | Description |
|---|---|
| `GMAIL_CLIENT_ID` | OAuth client ID (`…apps.googleusercontent.com`) |
| `GMAIL_CLIENT_SECRET` | OAuth client secret |

Variables can be set in the MCP client config `env` block, exported in your shell, or placed in a `.env` file in the working directory.

Environment variables take precedence over `credentials.json` if both are present.

### Config / data directory

The server stores `token.json` and `personal-email-style-guide.md` here:

| Platform | Primary | Fallback |
|---|---|---|
| Linux | `$XDG_CONFIG_HOME/gmail-mcp-server` (`~/.config/gmail-mcp-server`) | — |
| macOS | `~/.config/gmail-mcp-server` | `~/Library/Application Support/gmail-mcp-server` (used if it already exists) |
| Windows | `%AppData%\gmail-mcp-server` | — |

Run `./gmail-mcp-server --help` or the `/server-status` prompt to see the exact path on your machine.

---

## 4. MCP Client Setup

### stdio mode (simplest)

The MCP client launches a new server process each time. OAuth can prompt on every restart — use HTTP mode to avoid that.

**Claude Desktop** (`~/Library/Application Support/Claude/claude_desktop_config.json` on Mac,
`%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "gmail": {
      "command": "/path/to/gmail-mcp-server",
      "env": {
        "GMAIL_CLIENT_ID": "your_client_id.apps.googleusercontent.com",
        "GMAIL_CLIENT_SECRET": "your_client_secret"
      }
    }
  }
}
```

**Cursor** (`~/.cursor/mcp.json` on Mac/Linux, `%USERPROFILE%\.cursor\mcp.json` on Windows) — same format.

### HTTP mode (persistent, recommended)

Start the server once — OAuth happens at startup, then all clients share the same authenticated process:

```bash
# Default port 6633
./gmail-mcp-server --http

# Custom port
./gmail-mcp-server --http --port 3000
```

Then point your client at the MCP endpoint instead of a command:

```json
{
  "mcpServers": {
    "gmail": {
      "url": "http://localhost:6633/mcp"
    }
  }
}
```

Endpoint exposed in HTTP mode:
- `POST /mcp` — MCP JSON-RPC (streamable HTTP transport)

---

## 5. Tools and Resources

### Tools

| Tool | Description |
|---|---|
| `search_threads` | Search Gmail with Gmail query syntax + quarter shorthand |
| `fetch_email_bodies` | Fetch full body content for specific thread IDs |
| `create_draft` | Create a new draft or overwrite an existing one in a thread |
| `extract_attachment_by_filename` | Extract plain text from PDF, DOCX, or TXT attachments |
| `get_personal_email_style_guide` | Return the personal writing style guide |

### Quarter shorthand in searches

`search_threads` expands quarter references before querying Gmail:

| You write | Gmail receives |
|---|---|
| `Q1 2026` | `after:2026/01/01 before:2026/04/01` |
| `Q2 2026` | `after:2026/04/01 before:2026/07/01` |
| `Q3 2026` | `after:2026/07/01 before:2026/10/01` |
| `Q4 2025` | `after:2025/10/01 before:2026/01/01` |

Quarter references combine freely with other Gmail operators:

```
from:boss@company.com Q1 2026
has:attachment Q4 2025
subject:invoice Q2 2026 is:unread
```

### Resources

- `file://personal-email-style-guide` — your personal email writing style (auto-generated or manual)

### Prompts

- `/server-status` — show config paths and authentication status

---

## 6. Personal Email Style Guide

Create `personal-email-style-guide.md` in the config directory to give the AI your preferred tone and conventions. The file is returned by the `get_personal_email_style_guide` tool and the `file://personal-email-style-guide` resource before any draft is written.

---

## 7. Development

```bash
make build           # → bin/gmail-mcp-server
make install         # → $GOPATH/bin/gmail-mcp-server
make test            # race detector + coverage profile
make test-coverage   # test + HTML coverage report
make test-short      # skip slow/integration tests
make lint            # golangci-lint
make fmt             # gofmt + goimports
make tidy            # go mod tidy + verify

make release-snapshot  # local GoReleaser snapshot (no publish)
make release           # publish tagged release (requires GITHUB_TOKEN)
```

Cross-platform releases (Linux, macOS, Windows × amd64/arm64) are produced by [GoReleaser](.goreleaser.yaml).
