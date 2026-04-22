# Vitra MCP Server — Implementation Plan

## Overview

This document outlines the plan to build a **standalone MCP (Model Context Protocol) server** for the [Vitra](https://github.com/iamangus/vitra) note-taking application. The MCP server will be deployed as a **separate container** that mounts the same NFS share as the Vitra web app, enabling LLM clients (e.g., Claude, Cursor) to read, write, and manage markdown notes via MCP tools.

## Parent Project Context

**Vitra** is a lightweight, Obsidian-like note-taking web app written in Go. It serves a web UI and exposes a REST API for managing a vault of markdown files.

- **Repository**: `github.com/iamangus/vitra`
- **Language**: Go 1.23
- **Vault**: Directory of `.md` files with YAML frontmatter, wiki-links (`[[Note Name]]`), and tags (`#tag`)
- **Key files**:
  - `main.go` — HTTP server (port 8080), serves web UI + REST API
  - `api.go` — REST API handlers (`/api/*`)
  - `filesystem.go` — File tree builder (`FileSystem` struct, `buildTree`)
  - `markdown.go` — Markdown rendering, frontmatter parsing, wiki-link resolution
- **Deployment**: Containerized, vault path via `VAULT_PATH` env var (defaults to `./vault`)
- **Live reload**: Uses [Air](https://github.com/cosmtrek/air) for development

### Existing REST API (for reference)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/files` | List vault file tree |
| GET | `/api/note/{path...}` | Read note (content + frontmatter + html) |
| POST | `/api/note/{path...}` | Save note |
| POST | `/api/notes` | Create new note |
| POST | `/api/folders` | Create folder |
| PUT | `/api/rename` | Rename/move note or folder |
| DELETE | `/api/delete?path=` | Delete note or folder |
| GET | `/api/search?q=` | Search notes by content |
| GET | `/api/backlinks/{path...}` | Find notes linking to a given note |
| POST | `/api/preview` | Render markdown to HTML |

## Architecture Decision

### Separate Container, Shared NFS

The MCP server will be a **separate binary and container** from the Vitra web app. Both will mount the same `VAULT_PATH` (e.g., an NFS share). This means:

- **No shared code or memory** — the MCP server is a standalone Go program.
- **No coordination needed** — the filesystem is the single source of truth.
- **Changes are naturally visible** — if the MCP server writes a note, the web app sees it immediately (and vice versa) because they read from the same directory.

### Transport: Streamable HTTP

The MCP server will use the **Streamable HTTP** transport (MCP protocol 2025-03-26), not stdio. This allows remote LLM clients to connect over HTTP.

- **Endpoint**: `POST /mcp` and `GET /mcp` on a dedicated port (default 3000).
- **Library**: [`github.com/mark3labs/mcp-go`](https://github.com/mark3labs/mcp-go) — Go SDK for MCP servers.

## MCP Tools to Expose

The server will expose the following tools, mapping closely to the existing REST API:

| Tool | Input | Output | Description |
|------|-------|--------|-------------|
| `read_note` | `path` (string) | `content`, `frontmatter`, `html`, `title` | Read a note by vault-relative path |
| `write_note` | `path` (string), `content` (string) | success/error | Write or overwrite a note |
| `create_note` | `path` (string), `content` (string, optional) | success/error | Create a new note (fails if exists) |
| `delete_note` | `path` (string) | success/error | Delete a note or folder |
| `list_notes` | `path` (string, optional) | tree of notes/folders | List vault structure |
| `search_notes` | `query` (string) | list of matches | Search notes by content |
| `rename_note` | `old` (string), `new` (string) | success/error | Rename or move a note/folder |
| `get_backlinks` | `path` (string) | list of backlinks | Find notes linking to a given note |
| `create_folder` | `path` (string) | success/error | Create a new folder |

### Optional: MCP Resources

In addition to tools, we may expose **resources** (URI-addressable read-only data):

- `note://{path}` — individual note content
- `vault://tree` — full vault tree

This allows clients to discover and read notes via `resources/list` and `resources/read`, which is more idiomatic for read-only data in MCP.

## File Structure

```
vitra-mcp/
├── cmd/
│   └── mcp/
│       └── main.go          # Entrypoint: starts Streamable HTTP server
├── internal/
│   ├── server.go            # MCP server setup, tool registration
│   ├── tools.go             # Tool handler implementations
│   └── vault.go             # Vault filesystem operations (shared logic)
├── go.mod
├── go.sum
├── Dockerfile
├── docker-compose.yml       # Optional: both vitra + mcp services
└── README.md
```

## Implementation Steps

### 1. Initialize Go Module

```bash
go mod init github.com/iamangus/vitra-mcp
go get github.com/mark3labs/mcp-go
go get github.com/yuin/goldmark  # If reusing markdown rendering
```

### 2. Implement Vault Operations (`internal/vault.go`)

Re-implement or extract the core filesystem logic from Vitra:

- `ReadNote(path) -> (content, frontmatter, html, error)`
- `WriteNote(path, content) error`
- `CreateNote(path, content) error`
- `DeleteNote(path) error`
- `ListNotes(path) -> (tree, error)`
- `SearchNotes(query) -> (results, error)`
- `RenameNote(old, new) error`
- `GetBacklinks(path) -> (results, error)`
- `CreateFolder(path) error`

These should operate directly on `VAULT_PATH` using standard Go `os` and `path/filepath` packages.

### 3. Implement MCP Server (`internal/server.go`, `internal/tools.go`)

Using `mcp-go`:

```go
import (
    "github.com/mark3labs/mcp-go/mcp"
    "github.com/mark3labs/mcp-go/server"
)

// Create server
mcpServer := server.NewMCPServer(
    "vitra-mcp",
    "1.0.0",
    server.WithToolCapabilities(true),
)

// Register tools
mcpServer.AddTool(mcp.NewTool("read_note", ...), handleReadNote)
mcpServer.AddTool(mcp.NewTool("write_note", ...), handleWriteNote)
// ... etc

// Start HTTP server
httpServer := server.NewStreamableHTTPServer(mcpServer)
httpServer.Start(":3000")
```

### 4. Entrypoint (`cmd/mcp/main.go`)

```go
func main() {
    vaultPath := os.Getenv("VAULT_PATH")
    if vaultPath == "" {
        vaultPath = "./vault"
    }
    
    // Initialize vault and MCP server
    // Start on port 3000
}
```

### 5. Dockerfile

```dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o vitra-mcp ./cmd/mcp

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/vitra-mcp .
EXPOSE 3000
CMD ["./vitra-mcp"]
```

### 6. Docker Compose (Optional)

```yaml
version: '3.8'
services:
  vitra:
    image: vitra:latest
    ports:
      - "8080:8080"
    volumes:
      - nfs-vault:/vault
    environment:
      - VAULT_PATH=/vault

  vitra-mcp:
    image: vitra-mcp:latest
    ports:
      - "3000:3000"
    volumes:
      - nfs-vault:/vault
    environment:
      - VAULT_PATH=/vault

volumes:
  nfs-vault:
    driver: local
```

## Security Considerations

Per the MCP Streamable HTTP spec:

1. **Validate `Origin` header** on all incoming connections to prevent DNS rebinding.
2. **Bind to localhost** (`127.0.0.1`) inside the container; expose via reverse proxy (e.g., Nginx, Traefik) with authentication.
3. **Authentication**: Consider adding an API key or OAuth2 proxy in front of the MCP endpoint.
4. **Path traversal**: Sanitize all `path` inputs to prevent `../../../etc/passwd` attacks.
5. **Rate limiting**: Implement rate limiting on tool invocations.

## Open Questions

1. **Resources vs Tools**: Should we expose notes as MCP **resources** (read-only, URI-addressable) in addition to tools? This is more idiomatic for read operations.
2. **Authentication**: Do you need built-in auth (e.g., API key header), or is network-level security (VPN, reverse proxy) sufficient?
3. **Port**: Should the MCP server run on port 3000, or do you have a preference?
4. **Markdown rendering**: Should the MCP server include HTML rendering (like the web app), or return raw markdown only?
5. **Frontmatter**: Should tool outputs include parsed frontmatter as JSON, or just raw YAML?

## References

- [Vitra Repository](https://github.com/iamangus/vitra)
- [MCP Specification](https://modelcontextprotocol.io/specification/2025-03-26)
- [mcp-go Library](https://github.com/mark3labs/mcp-go)
- [MCP Streamable HTTP Transport](https://modelcontextprotocol.io/specification/2025-03-26/basic/transports#streamable-http)
