package internal

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/iamangus/vitra-mcp/internal/vector"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/renderer/html"
)

var (
	wikiLinkRegex = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)
	tagRegex      = regexp.MustCompile(`(?:^|\s)(#[\w\-/]+)`)
)

type Vault struct {
	Path        string
	VectorStore vector.VectorStore
}

func NewVault(path string, store vector.VectorStore) *Vault {
	return &Vault{Path: path, VectorStore: store}
}

type Note struct {
	Path        string                 `json:"path"`
	Title       string                 `json:"title"`
	Content     string                 `json:"content"`
	Frontmatter map[string]interface{} `json:"frontmatter"`
	HTML        string                 `json:"html,omitempty"`
}

type FileNode struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	IsDir    bool       `json:"is_dir"`
	Children []FileNode `json:"children,omitempty"`
}

func (v *Vault) ReadNote(path string) (*Note, error) {
	fullPath := filepath.Join(v.Path, path+".md")
	content, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("note not found: %s", path)
		}
		return nil, err
	}

	frontmatter, body := parseNote(content)
	html, _ := renderMarkdown(body, v.Path)

	return &Note{
		Path:        path,
		Title:       filepath.Base(path),
		Content:     string(content),
		Frontmatter: frontmatter,
		HTML:        html,
	}, nil
}

func (v *Vault) WriteNote(path string, content string) error {
	fullPath := filepath.Join(v.Path, path+".md")
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return err
	}

	// Auto-index in vector store
	if v.VectorStore != nil {
		chunks := vector.ChunkNote(path, content, 0, 0)
		if err := v.VectorStore.IndexNote(context.Background(), path, chunks); err != nil {
			// Log but don't fail the write
			fmt.Fprintf(os.Stderr, "Failed to index note %s: %v\n", path, err)
		}
	}

	return nil
}

func (v *Vault) CreateNote(path string, content string) error {
	fullPath := filepath.Join(v.Path, path+".md")
	if _, err := os.Stat(fullPath); err == nil {
		return fmt.Errorf("note already exists: %s", path)
	}

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	if content == "" {
		content = fmt.Sprintf("---\ntitle: %s\n---\n\n", filepath.Base(path))
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return err
	}

	// Auto-index in vector store
	if v.VectorStore != nil {
		chunks := vector.ChunkNote(path, content, 0, 0)
		if err := v.VectorStore.IndexNote(context.Background(), path, chunks); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to index note %s: %v\n", path, err)
		}
	}

	return nil
}

func (v *Vault) DeleteNote(path string) error {
	fullPath := filepath.Join(v.Path, path+".md")
	if err := os.RemoveAll(fullPath); err != nil {
		return err
	}

	// Remove from vector store
	if v.VectorStore != nil {
		if err := v.VectorStore.DeleteNote(context.Background(), path); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to delete note %s from index: %v\n", path, err)
		}
	}

	return nil
}

func (v *Vault) ListNotes(subpath string) ([]FileNode, error) {
	dir := v.Path
	if subpath != "" {
		dir = filepath.Join(v.Path, subpath)
	}
	return v.buildTree(dir, 0)
}

func (v *Vault) buildTree(dir string, depth int) ([]FileNode, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var nodes []FileNode
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}

		fullPath := filepath.Join(dir, name)
		relPath, _ := filepath.Rel(v.Path, fullPath)
		relPath = filepath.ToSlash(relPath)

		node := FileNode{
			Name:  strings.TrimSuffix(name, ".md"),
			Path:  strings.TrimSuffix(relPath, ".md"),
			IsDir: entry.IsDir(),
		}

		if entry.IsDir() {
			children, err := v.buildTree(fullPath, depth+1)
			if err == nil {
				node.Children = children
			}
		}

		nodes = append(nodes, node)
	}

	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].IsDir != nodes[j].IsDir {
			return nodes[i].IsDir
		}
		return strings.ToLower(nodes[i].Name) < strings.ToLower(nodes[j].Name)
	})

	return nodes, nil
}

func (v *Vault) SearchNotes(query string) ([]map[string]string, error) {
	query = strings.ToLower(query)
	var results []map[string]string

	err := filepath.Walk(v.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		if strings.Contains(strings.ToLower(string(content)), query) {
			rel, _ := filepath.Rel(v.Path, path)
			rel = strings.TrimSuffix(filepath.ToSlash(rel), ".md")
			results = append(results, map[string]string{
				"path":  rel,
				"title": strings.TrimSuffix(info.Name(), ".md"),
			})
		}
		return nil
	})

	return results, err
}

func (v *Vault) RenameNote(old, new string) error {
	oldFull := filepath.Join(v.Path, old)
	newFull := filepath.Join(v.Path, new)
	if err := os.Rename(oldFull, newFull); err != nil {
		return err
	}

	// Update vector store: delete old, index new
	if v.VectorStore != nil {
		if err := v.VectorStore.DeleteNote(context.Background(), old); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to delete old note %s from index: %v\n", old, err)
		}

		// Read new file and index it
		content, err := os.ReadFile(newFull)
		if err == nil {
			chunks := vector.ChunkNote(new, string(content), 0, 0)
			if err := v.VectorStore.IndexNote(context.Background(), new, chunks); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to index renamed note %s: %v\n", new, err)
			}
		}
	}

	return nil
}

func (v *Vault) GetBacklinks(path string) ([]map[string]string, error) {
	targetName := filepath.Base(path)
	var backlinks []map[string]string

	err := filepath.Walk(v.Path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}

		rel, _ := filepath.Rel(v.Path, filePath)
		rel = strings.TrimSuffix(filepath.ToSlash(rel), ".md")
		if rel == path {
			return nil
		}

		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil
		}

		pattern := "[[" + targetName + "]]"
		if strings.Contains(string(content), pattern) {
			backlinks = append(backlinks, map[string]string{
				"path":  rel,
				"title": strings.TrimSuffix(info.Name(), ".md"),
			})
		}
		return nil
	})

	return backlinks, err
}

func (v *Vault) CreateFolder(path string) error {
	fullPath := filepath.Join(v.Path, path)
	return os.MkdirAll(fullPath, 0755)
}

func renderMarkdown(content []byte, vaultPath string) (string, error) {
	processed := preprocessObsidianSyntax(content, vaultPath)
	var buf bytes.Buffer
	md := goldmark.New(
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
		),
	)
	if err := md.Convert(processed, &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func preprocessObsidianSyntax(content []byte, vaultPath string) []byte {
	text := string(content)
	var protected [][]string

	fencedRegex := regexp.MustCompile("(?s)```.*?```")
	text = fencedRegex.ReplaceAllStringFunc(text, func(match string) string {
		placeholder := fmt.Sprintf("\x00PROTECTED_%d\x00", len(protected))
		protected = append(protected, []string{placeholder, match})
		return placeholder
	})

	inlineCodeRegex := regexp.MustCompile("`[^`]+`")
	text = inlineCodeRegex.ReplaceAllStringFunc(text, func(match string) string {
		placeholder := fmt.Sprintf("\x00PROTECTED_%d\x00", len(protected))
		protected = append(protected, []string{placeholder, match})
		return placeholder
	})

	text = wikiLinkRegex.ReplaceAllStringFunc(text, func(match string) string {
		groups := wikiLinkRegex.FindStringSubmatch(match)
		if groups == nil {
			return match
		}
		target := strings.TrimSpace(groups[1])
		display := target
		if len(groups) > 2 && groups[2] != "" {
			display = strings.TrimSpace(groups[2])
		}
		targetPath := findNotePath(target, vaultPath)
		if targetPath != "" {
			return fmt.Sprintf(`<a href="/note/%s" class="wikilink">%s</a>`, targetPath, display)
		}
		return fmt.Sprintf(`<a href="/note/%s" class="wikilink missing">%s</a>`, targetPath, display)
	})

	text = tagRegex.ReplaceAllStringFunc(text, func(match string) string {
		groups := tagRegex.FindStringSubmatch(match)
		if groups == nil {
			return match
		}
		tag := groups[1]
		return fmt.Sprintf(` <a href="/search?q=%s" class="tag">%s</a>`, strings.TrimPrefix(tag, "#"), tag)
	})

	for i := len(protected) - 1; i >= 0; i-- {
		text = strings.Replace(text, protected[i][0], protected[i][1], 1)
	}

	return []byte(text)
}

func findNotePath(title string, vaultPath string) string {
	var found string
	filepath.Walk(vaultPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.EqualFold(strings.TrimSuffix(info.Name(), ".md"), title) {
			rel, _ := filepath.Rel(vaultPath, path)
			found = filepath.ToSlash(strings.TrimSuffix(rel, ".md"))
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func parseNote(content []byte) (frontmatter map[string]interface{}, body []byte) {
	frontmatter = make(map[string]interface{})
	body = content

	if !bytes.HasPrefix(content, []byte("---\n")) {
		return
	}

	parts := bytes.SplitN(content, []byte("---\n"), 3)
	if len(parts) < 3 {
		return
	}

	fmText := string(parts[1])
	lines := strings.Split(fmText, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			value = strings.Trim(value, `"'`)
			frontmatter[key] = value
		}
	}

	body = bytes.TrimSpace(parts[2])
	return
}
