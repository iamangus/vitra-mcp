package vector

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	// Match code blocks
	codeBlockRegex = regexp.MustCompile("(?s)```.*?```")
	// Match inline code
	inlineCodeRegex = regexp.MustCompile("`[^`]+`")
	// Match headers
	headerRegex = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
	// Match frontmatter
	frontmatterRegex = regexp.MustCompile("(?s)^---\n.*?\n---\n")
)

const (
	defaultChunkSize    = 1000
	defaultChunkOverlap = 200
)

// ChunkNote splits a markdown note into smart chunks with metadata injection
func ChunkNote(path string, content string, chunkSize int, chunkOverlap int) []Chunk {
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}
	if chunkOverlap <= 0 {
		chunkOverlap = defaultChunkOverlap
	}

	// Remove frontmatter
	content = frontmatterRegex.ReplaceAllString(content, "")

	// Extract and protect code blocks
	codeBlocks := make(map[string]string)
	content = codeBlockRegex.ReplaceAllStringFunc(content, func(match string) string {
		key := fmt.Sprintf("\x00CODE_BLOCK_%d\x00", len(codeBlocks))
		codeBlocks[key] = match
		return key
	})

	// Extract and protect inline code
	inlineCodes := make(map[string]string)
	content = inlineCodeRegex.ReplaceAllStringFunc(content, func(match string) string {
		key := fmt.Sprintf("\x00INLINE_CODE_%d\x00", len(inlineCodes))
		inlineCodes[key] = match
		return key
	})

	// Split into lines and process
	lines := strings.Split(content, "\n")
	var chunks []Chunk
	var currentChunk strings.Builder
	var currentHeading string
	var headingStack []string
	chunkIndex := 0

	flushChunk := func() {
		text := strings.TrimSpace(currentChunk.String())
		if text == "" {
			return
		}

		// Restore code blocks
		for key, val := range codeBlocks {
			text = strings.ReplaceAll(text, key, val)
		}
		for key, val := range inlineCodes {
			text = strings.ReplaceAll(text, key, val)
		}

		// Build heading breadcrumb
		heading := currentHeading
		if len(headingStack) > 0 {
			heading = strings.Join(headingStack, " > ")
		}

		// Inject metadata into text for embedding
		enhancedText := fmt.Sprintf("File: %s | Section: %s | %s", path, heading, text)

		chunks = append(chunks, Chunk{
			Text:    enhancedText,
			Index:   chunkIndex,
			Heading: heading,
			Path:    path,
		})
		chunkIndex++
		currentChunk.Reset()
	}

	for i, line := range lines {
		// Check if line is a header
		if matches := headerRegex.FindStringSubmatch(line); matches != nil {
			level := len(matches[1])
			title := strings.TrimSpace(matches[2])

			// Flush current chunk before starting new section
			if currentChunk.Len() > 0 {
				flushChunk()
			}

			// Update heading stack
			level-- // 0-indexed
			if level < len(headingStack) {
				headingStack = headingStack[:level]
			}
			headingStack = append(headingStack, title)
			currentHeading = title

			// Start new chunk with header
			currentChunk.WriteString(line)
			currentChunk.WriteString("\n")
			continue
		}

		// Add line to current chunk
		currentChunk.WriteString(line)
		currentChunk.WriteString("\n")

		// Check if we should split (at paragraph boundary or size limit)
		if currentChunk.Len() >= chunkSize {
			// Try to find a good split point (paragraph boundary)
			text := currentChunk.String()
			if idx := findSplitPoint(text, chunkSize, chunkOverlap); idx > 0 {
				// Split here
				first := text[:idx]
				second := text[idx:]

				currentChunk.Reset()
				currentChunk.WriteString(first)
				flushChunk()

				currentChunk.WriteString(second)
			} else {
				// Just flush and continue
				flushChunk()
			}
		}

		// Handle last chunk
		if i == len(lines)-1 && currentChunk.Len() > 0 {
			flushChunk()
		}
	}

	return chunks
}

// findSplitPoint finds a good place to split text, preferring paragraph boundaries
func findSplitPoint(text string, chunkSize int, overlap int) int {
	// Look for paragraph boundary before chunkSize
	searchEnd := chunkSize
	if searchEnd > len(text) {
		searchEnd = len(text)
	}

	// Search backwards for double newline (paragraph boundary)
	for i := searchEnd - 1; i > overlap; i-- {
		if i+1 < len(text) && text[i] == '\n' && text[i+1] == '\n' {
			return i + 2
		}
	}

	// Fallback: search for single newline
	for i := searchEnd - 1; i > overlap; i-- {
		if text[i] == '\n' {
			return i + 1
		}
	}

	return 0
}

// ChunkNoteSimple is a simpler version that just splits by size without markdown awareness
func ChunkNoteSimple(path string, content string, chunkSize int) []Chunk {
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}

	var chunks []Chunk
	contentBytes := []byte(content)

	for i := 0; i < len(contentBytes); i += chunkSize {
		end := i + chunkSize
		if end > len(contentBytes) {
			end = len(contentBytes)
		}

		text := string(contentBytes[i:end])
		enhancedText := fmt.Sprintf("File: %s | %s", path, text)

		chunks = append(chunks, Chunk{
			Text:    enhancedText,
			Index:   len(chunks),
			Heading: "",
			Path:    path,
		})
	}

	return chunks
}
