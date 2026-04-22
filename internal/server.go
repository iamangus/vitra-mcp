package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iamangus/vitra-mcp/internal/vector"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func StartMCPServer(vault *Vault, port string) error {
	s := server.NewMCPServer(
		"vitra-mcp",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	// read_note
	readNoteTool := mcp.NewTool("read_note",
		mcp.WithDescription("Read a note from the vault"),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Vault-relative path to the note (without .md extension)"),
		),
	)
	s.AddTool(readNoteTool, handleReadNote(vault))

	// write_note
	writeNoteTool := mcp.NewTool("write_note",
		mcp.WithDescription("Write or overwrite a note in the vault"),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Vault-relative path to the note (without .md extension)"),
		),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("Full markdown content of the note"),
		),
	)
	s.AddTool(writeNoteTool, handleWriteNote(vault))

	// create_note
	createNoteTool := mcp.NewTool("create_note",
		mcp.WithDescription("Create a new note (fails if it already exists)"),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Vault-relative path to the note (without .md extension)"),
		),
		mcp.WithString("content",
			mcp.Description("Optional initial content (defaults to frontmatter with title)"),
		),
	)
	s.AddTool(createNoteTool, handleCreateNote(vault))

	// delete_note
	deleteNoteTool := mcp.NewTool("delete_note",
		mcp.WithDescription("Delete a note or folder from the vault"),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Vault-relative path to delete"),
		),
	)
	s.AddTool(deleteNoteTool, handleDeleteNote(vault))

	// list_notes
	listNotesTool := mcp.NewTool("list_notes",
		mcp.WithDescription("List the vault file tree"),
		mcp.WithString("path",
			mcp.Description("Optional subpath to list (defaults to vault root)"),
		),
	)
	s.AddTool(listNotesTool, handleListNotes(vault))

	// search_notes
	searchNotesTool := mcp.NewTool("search_notes",
		mcp.WithDescription("Search notes by content"),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Search query string"),
		),
	)
	s.AddTool(searchNotesTool, handleSearchNotes(vault))

	// rename_note
	renameNoteTool := mcp.NewTool("rename_note",
		mcp.WithDescription("Rename or move a note or folder"),
		mcp.WithString("old",
			mcp.Required(),
			mcp.Description("Current vault-relative path"),
		),
		mcp.WithString("new",
			mcp.Required(),
			mcp.Description("New vault-relative path"),
		),
	)
	s.AddTool(renameNoteTool, handleRenameNote(vault))

	// get_backlinks
	getBacklinksTool := mcp.NewTool("get_backlinks",
		mcp.WithDescription("Find notes that link to a given note"),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Vault-relative path to the target note"),
		),
	)
	s.AddTool(getBacklinksTool, handleGetBacklinks(vault))

	// create_folder
	createFolderTool := mcp.NewTool("create_folder",
		mcp.WithDescription("Create a new folder in the vault"),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Vault-relative path to the new folder"),
		),
	)
	s.AddTool(createFolderTool, handleCreateFolder(vault))

	// search_wiki - semantic search
	searchWikiTool := mcp.NewTool("search_wiki",
		mcp.WithDescription("Semantic search across the vault using vector similarity"),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Search query"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of results (default: 5)"),
		),
	)
	s.AddTool(searchWikiTool, handleSearchWiki(vault))

	// find_similar_files
	findSimilarTool := mcp.NewTool("find_similar_files",
		mcp.WithDescription("Find files semantically similar to a given file"),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Vault-relative path to the reference note"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of results (default: 5)"),
		),
	)
	s.AddTool(findSimilarTool, handleFindSimilarFiles(vault))

	// suggest_links
	suggestLinksTool := mcp.NewTool("suggest_links",
		mcp.WithDescription("Suggest wiki-links for a note based on semantic similarity"),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Vault-relative path to the note"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of suggestions (default: 5)"),
		),
	)
	s.AddTool(suggestLinksTool, handleSuggestLinks(vault))

	// reindex_vault
	reindexVaultTool := mcp.NewTool("reindex_vault",
		mcp.WithDescription("Rebuild the entire vector index for the vault"),
	)
	s.AddTool(reindexVaultTool, handleReindexVault(vault))

	return server.NewStreamableHTTPServer(s).Start(":" + port)
}

func handleReadNote(vault *Vault) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := req.GetString("path", "")
		if path == "" {
			return nil, fmt.Errorf("path is required")
		}

		note, err := vault.ReadNote(path)
		if err != nil {
			return nil, err
		}

		data, err := json.MarshalIndent(note, "", "  ")
		if err != nil {
			return nil, err
		}

		return mcp.NewToolResultText(string(data)), nil
	}
}

func handleWriteNote(vault *Vault) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := req.GetString("path", "")
		content := req.GetString("content", "")
		if path == "" {
			return nil, fmt.Errorf("path is required")
		}

		if err := vault.WriteNote(path, content); err != nil {
			return nil, err
		}

		return mcp.NewToolResultText(fmt.Sprintf("Note written: %s", path)), nil
	}
}

func handleCreateNote(vault *Vault) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := req.GetString("path", "")
		content := req.GetString("content", "")
		if path == "" {
			return nil, fmt.Errorf("path is required")
		}

		if err := vault.CreateNote(path, content); err != nil {
			return nil, err
		}

		return mcp.NewToolResultText(fmt.Sprintf("Note created: %s", path)), nil
	}
}

func handleDeleteNote(vault *Vault) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := req.GetString("path", "")
		if path == "" {
			return nil, fmt.Errorf("path is required")
		}

		if err := vault.DeleteNote(path); err != nil {
			return nil, err
		}

		return mcp.NewToolResultText(fmt.Sprintf("Deleted: %s", path)), nil
	}
}

func handleListNotes(vault *Vault) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := req.GetString("path", "")

		tree, err := vault.ListNotes(path)
		if err != nil {
			return nil, err
		}

		data, err := json.MarshalIndent(tree, "", "  ")
		if err != nil {
			return nil, err
		}

		return mcp.NewToolResultText(string(data)), nil
	}
}

func handleSearchNotes(vault *Vault) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query := req.GetString("query", "")
		if query == "" {
			return nil, fmt.Errorf("query is required")
		}

		results, err := vault.SearchNotes(query)
		if err != nil {
			return nil, err
		}

		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return nil, err
		}

		return mcp.NewToolResultText(string(data)), nil
	}
}

func handleRenameNote(vault *Vault) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		old := req.GetString("old", "")
		new := req.GetString("new", "")
		if old == "" || new == "" {
			return nil, fmt.Errorf("old and new paths are required")
		}

		if err := vault.RenameNote(old, new); err != nil {
			return nil, err
		}

		return mcp.NewToolResultText(fmt.Sprintf("Renamed: %s -> %s", old, new)), nil
	}
}

func handleGetBacklinks(vault *Vault) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := req.GetString("path", "")
		if path == "" {
			return nil, fmt.Errorf("path is required")
		}

		results, err := vault.GetBacklinks(path)
		if err != nil {
			return nil, err
		}

		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return nil, err
		}

		return mcp.NewToolResultText(string(data)), nil
	}
}

func handleCreateFolder(vault *Vault) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := req.GetString("path", "")
		if path == "" {
			return nil, fmt.Errorf("path is required")
		}

		if err := vault.CreateFolder(path); err != nil {
			return nil, err
		}

		return mcp.NewToolResultText(fmt.Sprintf("Folder created: %s", path)), nil
	}
}

func handleSearchWiki(vault *Vault) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query := req.GetString("query", "")
		if query == "" {
			return nil, fmt.Errorf("query is required")
		}

		limit := int(req.GetFloat("limit", 5))

		if vault.VectorStore == nil {
			return nil, fmt.Errorf("vector store not configured")
		}

		results, err := vault.VectorStore.SemanticSearch(ctx, query, limit)
		if err != nil {
			return nil, fmt.Errorf("semantic search failed: %w", err)
		}

		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return nil, err
		}

		return mcp.NewToolResultText(string(data)), nil
	}
}

func handleFindSimilarFiles(vault *Vault) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := req.GetString("path", "")
		if path == "" {
			return nil, fmt.Errorf("path is required")
		}

		limit := int(req.GetFloat("limit", 5))

		if vault.VectorStore == nil {
			return nil, fmt.Errorf("vector store not configured")
		}

		results, err := vault.VectorStore.FindSimilarFiles(ctx, path, limit)
		if err != nil {
			return nil, fmt.Errorf("find similar files failed: %w", err)
		}

		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return nil, err
		}

		return mcp.NewToolResultText(string(data)), nil
	}
}

func handleSuggestLinks(vault *Vault) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := req.GetString("path", "")
		if path == "" {
			return nil, fmt.Errorf("path is required")
		}

		limit := int(req.GetFloat("limit", 5))

		if vault.VectorStore == nil {
			return nil, fmt.Errorf("vector store not configured")
		}

		// Use the note path to find similar files
		results, err := vault.VectorStore.FindSimilarFiles(ctx, path, limit)
		if err != nil {
			return nil, fmt.Errorf("suggest links failed: %w", err)
		}

		// Format as suggestions
		suggestions := make([]map[string]string, len(results))
		for i, r := range results {
			suggestions[i] = map[string]string{
				"path":    r.Path,
				"title":   r.Title,
				"heading": r.Heading,
				"link":    fmt.Sprintf("[[%s]]", r.Title),
			}
		}

		data, err := json.MarshalIndent(suggestions, "", "  ")
		if err != nil {
			return nil, err
		}

		return mcp.NewToolResultText(string(data)), nil
	}
}

func handleReindexVault(vault *Vault) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if vault.VectorStore == nil {
			return nil, fmt.Errorf("vector store not configured")
		}

		// Clear the index
		if err := vault.VectorStore.ReindexVault(ctx, vault.Path); err != nil {
			return nil, fmt.Errorf("failed to clear index: %w", err)
		}

		// Walk the vault and index all notes
		var indexed int
		err := filepath.Walk(vault.Path, func(filePath string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".md") {
				return nil
			}

			rel, _ := filepath.Rel(vault.Path, filePath)
			rel = strings.TrimSuffix(filepath.ToSlash(rel), ".md")

			content, err := os.ReadFile(filePath)
			if err != nil {
				return nil
			}

			chunks := vector.ChunkNote(rel, string(content), 0, 0)
			if err := vault.VectorStore.IndexNote(ctx, rel, chunks); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to index %s: %v\n", rel, err)
			} else {
				indexed++
			}
			return nil
		})

		if err != nil {
			return nil, err
		}

		return mcp.NewToolResultText(fmt.Sprintf("Reindexed %d notes", indexed)), nil
	}
}
