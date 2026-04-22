# Vitra MCP Server

A standalone [Model Context Protocol (MCP)](https://modelcontextprotocol.io) server for the [Vitra](https://github.com/iamangus/vitra) note-taking application.

## Overview

This MCP server exposes tools to read, write, and manage markdown notes in a Vitra vault. It is designed to run as a **separate container** that mounts the same filesystem (e.g., NFS share) as the Vitra web app.

## Tools

### File Operations

| Tool | Description |
|------|-------------|
| `read_note` | Read a note by path |
| `write_note` | Write or overwrite a note |
| `create_note` | Create a new note (fails if exists) |
| `delete_note` | Delete a note or folder |
| `list_notes` | List vault file tree |
| `search_notes` | Search notes by content |
| `rename_note` | Rename or move a note/folder |
| `get_backlinks` | Find notes linking to a given note |
| `create_folder` | Create a new folder |

### Vector Search (ChromaDB)

| Tool | Description |
|------|-------------|
| `search_wiki` | Semantic search across the vault using vector similarity |
| `find_similar_files` | Find files semantically similar to a given file |
| `suggest_links` | Suggest wiki-links for a note based on semantic similarity |
| `reindex_vault` | Rebuild the entire vector index for the vault |

## Configuration

### Required

- `VAULT_PATH` — Path to the markdown vault (default: `./vault`)
- `PORT` — HTTP port for MCP server (default: `3000`)

### ChromaDB

- `CHROMADB_URL` — ChromaDB server URL (default: `http://localhost:8000`)

### Embeddings (OpenAI-compatible)

- `EMBEDDING_API_URL` — Embedding API base URL (default: `https://api.openai.com/v1`)
- `EMBEDDING_API_KEY` — API key for embedding service
- `EMBEDDING_MODEL` — Model name (default: `text-embedding-3-small`)

## Running

### Prerequisites

For vector search features, you need either:
- **ChromaDB** running (via Docker Compose or standalone)
- **OpenAI API key** or compatible embedding service

### Option 1: Docker Compose (Recommended)

Starts ChromaDB + MCP server together:

```bash
export EMBEDDING_API_URL=https://api.openai.com/v1
export EMBEDDING_API_KEY=your-key-here

docker-compose up
```

The MCP server will be available at `http://localhost:3000`.

### Option 2: ChromaDB via Docker, MCP Server Locally

Good for development:

```bash
# Terminal 1: Start ChromaDB
docker run -p 8000:8000 chromadb/chroma:latest

# Terminal 2: Run the MCP server
cd /home/angoo/repos/vitra-mcp
export VAULT_PATH=./vault
export PORT=3000
export CHROMADB_URL=http://localhost:8000
export EMBEDDING_API_URL=https://api.openai.com/v1
export EMBEDDING_API_KEY=your-key-here
go run ./cmd/mcp
```

### Option 3: File Operations Only (No Vector Search)

For testing basic file operations without ChromaDB:

```bash
cd /home/angoo/repos/vitra-mcp
export VAULT_PATH=./vault
export PORT=3000
go run ./cmd/mcp
```

Vector search tools will return "vector store not configured" but all file operations work.

### With Docker

```bash
docker build -t vitra-mcp .
docker run -e VAULT_PATH=/vault \
  -e CHROMADB_URL=http://host.docker.internal:8000 \
  -e EMBEDDING_API_KEY=your-key-here \
  -v /path/to/vault:/vault \
  -p 3000:3000 \
  vitra-mcp
```

## Testing

Once running, test with curl:

```bash
# Check if server is up
curl http://localhost:3000/mcp

# List tools (via MCP protocol)
curl -X POST http://localhost:3000/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

Or connect via an MCP client like Claude Desktop, Cline, etc.

## Architecture

- **Separate container** from the Vitra web app
- **Shared filesystem** (NFS) — no shared memory or coordination needed
- **Streamable HTTP** transport on port 3000
- **Vector store** via ChromaDB for semantic search
- **Auto-indexing** — notes are automatically indexed on write/create/delete/rename
- **Markdown-aware chunking** — respects headers, code blocks, and frontmatter
- Endpoint: `POST /mcp` and `GET /mcp`

## Vector Store Features

### Auto-Indexing
Every write, create, delete, or rename operation automatically updates the vector index.

### Smart Chunking
- Splits by markdown headers first
- Never splits inside code blocks
- Injects metadata into embeddings: `File: path | Section: heading | content`
- Configurable chunk size and overlap

### Deduplication
Built-in similarity checking warns if content is too similar to existing notes.
