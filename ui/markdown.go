package ui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"claude-squad/log"
)

// RenderMarkdown renders markdown content for terminal display
// Returns the rendered string and any error that occurred
func RenderMarkdown(content string, width int) (string, error) {
	// Create a custom glamour renderer with appropriate styling
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
		glamour.WithEmoji(),
	)
	if err != nil {
		log.ErrorLog.Printf("Failed to create markdown renderer: %v", err)
		return content, err
	}

	// Render the markdown
	rendered, err := r.Render(content)
	if err != nil {
		log.ErrorLog.Printf("Failed to render markdown: %v", err)
		return content, err
	}

	// Remove trailing newlines that glamour adds
	rendered = strings.TrimRight(rendered, "\n")

	return rendered, nil
}

// RenderMarkdownWithStyle renders markdown with custom styling options
func RenderMarkdownWithStyle(content string, width int, isDark bool) (string, error) {
	styleName := "dark"
	if !isDark {
		styleName = "light"
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStylePath(styleName),
		glamour.WithWordWrap(width),
		glamour.WithEmoji(),
	)
	if err != nil {
		log.ErrorLog.Printf("Failed to create markdown renderer with style: %v", err)
		return content, err
	}

	rendered, err := r.Render(content)
	if err != nil {
		log.ErrorLog.Printf("Failed to render markdown with style: %v", err)
		return content, err
	}

	// Remove trailing newlines
	rendered = strings.TrimRight(rendered, "\n")

	return rendered, nil
}

// StripMarkdown removes markdown formatting from text, useful for previews
func StripMarkdown(content string) string {
	// Basic markdown stripping - this is simplified and won't handle all cases
	// For production, consider using a proper markdown parser
	
	// Remove code blocks
	content = strings.ReplaceAll(content, "```", "")
	
	// Remove inline code
	content = strings.ReplaceAll(content, "`", "")
	
	// Remove bold and italic markers
	content = strings.ReplaceAll(content, "**", "")
	content = strings.ReplaceAll(content, "__", "")
	content = strings.ReplaceAll(content, "*", "")
	content = strings.ReplaceAll(content, "_", "")
	
	// Remove headers
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			// Remove the # symbols and any following space
			for strings.HasPrefix(trimmed, "#") {
				trimmed = strings.TrimPrefix(trimmed, "#")
			}
			lines[i] = strings.TrimSpace(trimmed)
		}
	}
	content = strings.Join(lines, "\n")
	
	// Remove links but keep text
	// [text](url) -> text
	for strings.Contains(content, "](") {
		start := strings.Index(content, "[")
		if start == -1 {
			break
		}
		end := strings.Index(content[start:], "](")
		if end == -1 {
			break
		}
		end += start
		closeIdx := strings.Index(content[end:], ")")
		if closeIdx == -1 {
			break
		}
		closeIdx += end
		
		linkText := content[start+1 : end]
		content = content[:start] + linkText + content[closeIdx+1:]
	}
	
	return content
}

// RenderMarkdownInBox renders markdown content inside a lipgloss box
func RenderMarkdownInBox(content string, width int, boxStyle lipgloss.Style) string {
	// Account for box padding and borders
	innerWidth := width - boxStyle.GetHorizontalFrameSize()
	if innerWidth < 20 {
		innerWidth = 20
	}

	rendered, err := RenderMarkdown(content, innerWidth)
	if err != nil {
		// Fallback to plain text
		rendered = content
	}

	return boxStyle.Render(rendered)
}