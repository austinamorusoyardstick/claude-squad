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
	// Basic formatting
	inlineCodeRegex       = regexp.MustCompile("`([^`]+)`")
	boldRegex             = regexp.MustCompile(`(\*\*|__)([^*_]+)(\*\*|__)`)
	singleAsteriskRegex   = regexp.MustCompile(`(?:^|\s)\*([^*\n]+)\*(?:\s|$)`)
	singleUnderscoreRegex = regexp.MustCompile(`(?:^|\s)_([^_\n]+)_(?:\s|$)`)
	strikethroughRegex    = regexp.MustCompile(`~~([^~]+)~~`)
	
	// Lists
	unorderedListRegex = regexp.MustCompile(`^(\s*)([-*+])\s+(.*)`)
	orderedListRegex   = regexp.MustCompile(`^(\s*)(\d+)\.\s+(.*)`)
	
	// Links and images
	linkRegex  = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	imageRegex = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)
	
	// Block elements
	blockquoteRegex = regexp.MustCompile(`^>\s*(.*)`)
	hrRegex         = regexp.MustCompile(`^(---|\*\*\*|___)$`)
	headerRegex     = regexp.MustCompile(`^(#{1,6})\s+(.*)`)
	
	// Code blocks
	codeBlockStartRegex = regexp.MustCompile(`^` + "```" + `(\w*)`)
	codeBlockEndRegex   = regexp.MustCompile(`^` + "```" + `$`)
	
	// Tables
	tableRowRegex = regexp.MustCompile(`^\|(.+)\|$`)
	tableSepRegex = regexp.MustCompile(`^\|[\s\-:|]+\|$`)
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

// RenderMarkdownLight provides a lightweight markdown rendering with enhanced features
// This is much faster than full glamour rendering and suitable for real-time updates
func RenderMarkdownLight(content string) string {
	// Define styles
	boldStyle := lipgloss.NewStyle().Bold(true)
	italicStyle := lipgloss.NewStyle().Italic(true)
	codeStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("213"))
	strikethroughStyle := lipgloss.NewStyle().Strikethrough(true)
	linkStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Underline(true)
	blockquoteStyle := lipgloss.NewStyle().
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("241")).
		PaddingLeft(1).
		Foreground(lipgloss.Color("245"))
	hrStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))
	
	// Header styles
	h1Style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	h2Style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213"))
	h3Style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("221"))
	h4Style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("229"))
	h5Style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("237"))
	h6Style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245"))
	
	// Process line by line
	lines := strings.Split(content, "\n")
	processedLines := make([]string, 0, len(lines))
	
	inCodeBlock := false
	codeBlockLang := ""
	codeBlockLines := []string{}
	inTable := false
	tableRows := [][]string{}
	
	for i, line := range lines {
		// Handle code blocks
		if match := codeBlockStartRegex.FindStringSubmatch(line); match != nil && !inCodeBlock {
			inCodeBlock = true
			codeBlockLang = match[1]
			continue
		}
		
		if codeBlockEndRegex.MatchString(line) && inCodeBlock {
			inCodeBlock = false
			codeContent := strings.Join(codeBlockLines, "\n")
			
			// Add language hint if available
			if codeBlockLang != "" {
				langHint := lipgloss.NewStyle().
					Foreground(lipgloss.Color("243")).
					Render("▎" + codeBlockLang)
				processedLines = append(processedLines, langHint)
			}
			
			// Render code block with indentation
			for _, codeLine := range codeBlockLines {
				processedLines = append(processedLines, "  "+codeStyle.Render(codeLine))
			}
			
			codeBlockLines = nil
			codeBlockLang = ""
			continue
		}
		
		if inCodeBlock {
			codeBlockLines = append(codeBlockLines, line)
			continue
		}
		
		// Handle tables
		if tableRowRegex.MatchString(line) {
			if !inTable {
				inTable = true
				tableRows = [][]string{}
			}
			
			// Skip separator rows but use them to detect tables
			if tableSepRegex.MatchString(line) {
				continue
			}
			
			// Parse table row
			cells := strings.Split(strings.Trim(line, "|"), "|")
			for j := range cells {
				cells[j] = strings.TrimSpace(cells[j])
			}
			tableRows = append(tableRows, cells)
			
			// Check if next line is not a table row to end the table
			if i == len(lines)-1 || (i < len(lines)-1 && !tableRowRegex.MatchString(lines[i+1])) {
				// Render the table
				processedLines = append(processedLines, renderSimpleTable(tableRows)...)
				inTable = false
				tableRows = nil
			}
			continue
		}
		
		// Handle horizontal rules
		if hrRegex.MatchString(strings.TrimSpace(line)) {
			processedLines = append(processedLines, hrStyle.Render(strings.Repeat("─", 40)))
			continue
		}
		
		// Handle headers with different styles
		if match := headerRegex.FindStringSubmatch(line); match != nil {
			level := len(match[1])
			headerText := match[2]
			
			var style lipgloss.Style
			switch level {
			case 1:
				style = h1Style
			case 2:
				style = h2Style
			case 3:
				style = h3Style
			case 4:
				style = h4Style
			case 5:
				style = h5Style
			default:
				style = h6Style
			}
			
			processedLines = append(processedLines, style.Render(headerText))
			// Add spacing after headers
			if i < len(lines)-1 && lines[i+1] != "" {
				processedLines = append(processedLines, "")
			}
			continue
		}
		
		// Handle blockquotes
		if match := blockquoteRegex.FindStringSubmatch(line); match != nil {
			quotedText := match[1]
			// Process inline formatting in blockquote
			quotedText = processInlineFormatting(quotedText, boldStyle, italicStyle, codeStyle, strikethroughStyle, linkStyle)
			processedLines = append(processedLines, blockquoteStyle.Render(quotedText))
			continue
		}
		
		// Handle lists with better formatting
		if match := unorderedListRegex.FindStringSubmatch(line); match != nil {
			indent := match[1]
			content := match[3]
			
			// Calculate bullet based on indentation level
			indentLevel := len(indent) / 2
			bullets := []string{"•", "◦", "▪", "▫"}
			bullet := bullets[indentLevel%len(bullets)]
			
			// Process inline formatting in list item
			content = processInlineFormatting(content, boldStyle, italicStyle, codeStyle, strikethroughStyle, linkStyle)
			processedLines = append(processedLines, indent+bullet+" "+content)
			continue
		}
		
		if match := orderedListRegex.FindStringSubmatch(line); match != nil {
			indent := match[1]
			number := match[2]
			content := match[3]
			
			// Process inline formatting in list item
			content = processInlineFormatting(content, boldStyle, italicStyle, codeStyle, strikethroughStyle, linkStyle)
			processedLines = append(processedLines, indent+number+". "+content)
			continue
		}
		
		// Process inline formatting for regular lines
		processedLine := processInlineFormatting(line, boldStyle, italicStyle, codeStyle, strikethroughStyle, linkStyle)
		processedLines = append(processedLines, processedLine)
	}
	
	// Handle any unclosed code block
	if inCodeBlock && len(codeBlockLines) > 0 {
		codeContent := strings.Join(codeBlockLines, "\n")
		for _, codeLine := range strings.Split(codeContent, "\n") {
			processedLines = append(processedLines, "  "+codeStyle.Render(codeLine))
		}
	}
	
	return strings.Join(processedLines, "\n")
}

// processInlineFormatting handles inline markdown formatting
func processInlineFormatting(line string, boldStyle, italicStyle, codeStyle, strikethroughStyle, linkStyle lipgloss.Style) string {
	// Handle inline code first (to avoid conflicts)
	line = inlineCodeRegex.ReplaceAllStringFunc(line, func(match string) string {
		code := strings.Trim(match, "`")
		return codeStyle.Render(code)
	})
	
	// Handle links
	line = linkRegex.ReplaceAllStringFunc(line, func(match string) string {
		submatch := linkRegex.FindStringSubmatch(match)
		if len(submatch) > 2 {
			text := submatch[1]
			url := submatch[2]
			// Show both text and URL in a nice format
			return linkStyle.Render(text) + " " + lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Render("("+url+")")
		}
		return match
	})
	
	// Handle images (show as [Image: description])
	line = imageRegex.ReplaceAllStringFunc(line, func(match string) string {
		submatch := imageRegex.FindStringSubmatch(match)
		if len(submatch) > 1 {
			alt := submatch[1]
			if alt == "" {
				alt = "image"
			}
			return lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render("[Image: " + alt + "]")
		}
		return match
	})
	
	// Handle strikethrough
	line = strikethroughRegex.ReplaceAllStringFunc(line, func(match string) string {
		text := strikethroughRegex.FindStringSubmatch(match)[1]
		return strikethroughStyle.Render(text)
	})
	
	// Handle bold
	line = boldRegex.ReplaceAllStringFunc(line, func(match string) string {
		text := boldRegex.FindStringSubmatch(match)[2]
		return boldStyle.Render(text)
	})
	
	// Handle italic with asterisks
	line = singleAsteriskRegex.ReplaceAllStringFunc(line, func(match string) string {
		submatch := singleAsteriskRegex.FindStringSubmatch(match)
		if len(submatch) > 1 {
			text := submatch[1]
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
	
	// Handle italic with underscores
	line = singleUnderscoreRegex.ReplaceAllStringFunc(line, func(match string) string {
		submatch := singleUnderscoreRegex.FindStringSubmatch(match)
		if len(submatch) > 1 {
			text := submatch[1]
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
	
	return line
}