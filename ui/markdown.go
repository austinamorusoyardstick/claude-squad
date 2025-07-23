package ui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"claude-squad/log"
)

// Pre-compiled regexes for better performance
var (
	inlineCodeRegex      = regexp.MustCompile("`([^`]+)`")
	boldRegex            = regexp.MustCompile(`(\*\*|__)([^*_]+)(\*\*|__)`)
	singleAsteriskRegex  = regexp.MustCompile(`(?:^|\s)\*([^*\n]+)\*(?:\s|$)`)
	singleUnderscoreRegex = regexp.MustCompile(`(?:^|\s)_([^_\n]+)_(?:\s|$)`)
	listItemRegex        = regexp.MustCompile(`^\s*\d+\.`)
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

// RenderMarkdownLight provides a lightweight markdown rendering that only handles basic formatting
// This is much faster than full glamour rendering and suitable for real-time updates
func RenderMarkdownLight(content string) string {
	// Define styles
	boldStyle := lipgloss.NewStyle().Bold(true)
	italicStyle := lipgloss.NewStyle().Italic(true)
	codeStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("213"))
	
	// Process line by line
	lines := strings.Split(content, "\n")
	processedLines := make([]string, 0, len(lines))
	
	inCodeBlock := false
	codeBlockLines := []string{}
	
	for _, line := range lines {
		// Handle code blocks
		if strings.HasPrefix(line, "```") {
			if inCodeBlock {
				// End code block
				inCodeBlock = false
				codeContent := strings.Join(codeBlockLines, "\n")
				processedLines = append(processedLines, codeStyle.Render(codeContent))
				codeBlockLines = nil
			} else {
				// Start code block
				inCodeBlock = true
			}
			continue
		}
		
		if inCodeBlock {
			codeBlockLines = append(codeBlockLines, line)
			continue
		}
		
		// Handle headers (simple approach - just make them bold)
		if strings.HasPrefix(line, "#") {
			headerText := strings.TrimLeft(line, "# ")
			processedLines = append(processedLines, boldStyle.Render(headerText))
			continue
		}
		
		// Process inline formatting
		processedLine := line
		
		// Handle inline code (must be done before bold/italic to avoid conflicts)
		processedLine = inlineCodeRegex.ReplaceAllStringFunc(processedLine, func(match string) string {
			code := strings.Trim(match, "`")
			return codeStyle.Render(code)
		})
		
		// Handle bold (**text** or __text__)
		processedLine = boldRegex.ReplaceAllStringFunc(processedLine, func(match string) string {
			text := boldRegex.FindStringSubmatch(match)[2]
			return boldStyle.Render(text)
		})
		
		// Handle italic (*text* or _text_) - simpler approach
		// Handle single asterisks
		processedLine = singleAsteriskRegex.ReplaceAllStringFunc(processedLine, func(match string) string {
			submatch := singleAsteriskRegex.FindStringSubmatch(match)
			if len(submatch) > 1 {
				text := submatch[1]
				// Preserve whitespace
				prefix := ""
				suffix := ""
				if strings.HasPrefix(match, " ") {
					prefix = " "
				}
				if strings.HasSuffix(match, " ") {
					suffix = " "
				}
				return prefix + italicStyle.Render(text) + suffix
			}
			return match
		})
		
		// Handle single underscores
		processedLine = singleUnderscoreRegex.ReplaceAllStringFunc(processedLine, func(match string) string {
			submatch := singleUnderscoreRegex.FindStringSubmatch(match)
			if len(submatch) > 1 {
				text := submatch[1]
				// Preserve whitespace
				prefix := ""
				suffix := ""
				if strings.HasPrefix(match, " ") {
					prefix = " "
				}
				if strings.HasSuffix(match, " ") {
					suffix = " "
				}
				return prefix + italicStyle.Render(text) + suffix
			}
			return match
		})
		
		// Handle lists (just add some indentation)
		if strings.HasPrefix(strings.TrimSpace(line), "- ") ||
			strings.HasPrefix(strings.TrimSpace(line), "* ") ||
			listItemRegex.MatchString(line) {
			processedLine = "  • " + strings.TrimSpace(processedLine[2:])
		}
		
		processedLines = append(processedLines, processedLine)
	}
	
	// Handle any unclosed code block
	if inCodeBlock && len(codeBlockLines) > 0 {
		codeContent := strings.Join(codeBlockLines, "\n")
		processedLines = append(processedLines, codeStyle.Render(codeContent))
	}
	
	return strings.Join(processedLines, "\n")
}