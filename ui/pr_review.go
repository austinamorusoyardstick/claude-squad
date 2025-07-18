package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"claude-squad/session/git"
)

type PRReviewModel struct {
	pr            *git.PullRequest
	currentIndex  int
	width         int
	height        int
	showHelp      bool
	err           error
	viewport      viewport.Model
	ready         bool
}

type PRReviewCompleteMsg struct {
	AcceptedComments []git.PRComment
}

type PRReviewCancelMsg struct{}

func NewPRReviewModel(pr *git.PullRequest) PRReviewModel {
	return PRReviewModel{
		pr:           pr,
		currentIndex: 0,
		showHelp:     true,
		ready:        false,
		width:        80,  // Default width
		height:       24,  // Default height
	}
}

func (m PRReviewModel) Init() tea.Cmd {
	// Request window size immediately
	return tea.WindowSize()
}

func (m PRReviewModel) Update(msg tea.Msg) (PRReviewModel, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		
		// Header: title (1) + status (1) + blank line after status (1) + blank line before viewport (1) = 4
		headerHeight := 4
		// Footer: blank line before help (1) + help text (1) = 2
		footerHeight := 2
		if !m.showHelp {
			footerHeight = 0
		}
		
		// Add a larger safety margin to account for:
		// - Terminal status lines
		// - Potential rendering artifacts
		// - Any framework overhead
		safetyMargin := 3
		
		if !m.ready {
			m.viewport = viewport.New(m.width, m.height-headerHeight-footerHeight-safetyMargin)
			m.viewport.HighPerformanceRendering = false
			m.ready = true
			m.viewport.SetYOffset(0)
		} else {
			m.viewport.Width = m.width
			m.viewport.Height = m.height - headerHeight - footerHeight - safetyMargin
		}
		
		m.updateViewportContent()
		m.viewport.SetYOffset(0) // Reset scroll position
		return m, nil

	case tea.KeyMsg:
		// Handle key events even when not ready
		switch msg.String() {
		case "q", "esc":
			return m, func() tea.Msg { return PRReviewCancelMsg{} }
		
		case "?":
			m.showHelp = !m.showHelp
			// Adjust viewport height when help is toggled if ready
			if m.ready {
				headerHeight := 4
				footerHeight := 2
				if !m.showHelp {
					footerHeight = 0
				}
				safetyMargin := 3
				m.viewport.Height = m.height - headerHeight - footerHeight - safetyMargin
				m.updateViewportContent()
			}
			return m, nil
		
		case "j", "down":
			if m.currentIndex < len(m.pr.Comments)-1 {
				m.currentIndex++
				if m.ready {
					m.updateViewportContent()
					m.ensureCurrentCommentVisible()
				}
			}
			return m, nil
		
		case "k", "up":
			if m.currentIndex > 0 {
				m.currentIndex--
				if m.ready {
					m.updateViewportContent()
					m.ensureCurrentCommentVisible()
				}
			}
			return m, nil
		
		case "a":
			if len(m.pr.Comments) > 0 {
				m.pr.Comments[m.currentIndex].Accepted = true
				if m.ready {
					m.updateViewportContent()
				}
			}
			return m, nil
		
		case "d":
			if len(m.pr.Comments) > 0 {
				m.pr.Comments[m.currentIndex].Accepted = false
				if m.ready {
					m.updateViewportContent()
				}
			}
			return m, nil
		
		case "A":
			for i := range m.pr.Comments {
				m.pr.Comments[i].Accepted = true
			}
			if m.ready {
				m.updateViewportContent()
			}
			return m, nil
		
		case "D":
			for i := range m.pr.Comments {
				m.pr.Comments[i].Accepted = false
			}
			if m.ready {
				m.updateViewportContent()
			}
			return m, nil
		
		case "enter":
			acceptedComments := m.pr.GetAcceptedComments()
			return m, func() tea.Msg { return PRReviewCompleteMsg{AcceptedComments: acceptedComments} }
		
		// Additional viewport controls (only when ready)
		case "pgup", "shift+up":
			if m.ready {
				m.viewport.ViewUp()
			}
			return m, nil
		
		case "pgdown", "shift+down":
			if m.ready {
				m.viewport.ViewDown()
			}
			return m, nil
		
		case "home", "g":
			if m.ready {
				m.viewport.GotoTop()
			}
			if len(m.pr.Comments) > 0 {
				m.currentIndex = 0
				if m.ready {
					m.updateViewportContent()
				}
			}
			return m, nil
		
		case "end", "G":
			if m.ready {
				m.viewport.GotoBottom()
			}
			if len(m.pr.Comments) > 0 {
				m.currentIndex = len(m.pr.Comments) - 1
				if m.ready {
					m.updateViewportContent()
				}
			}
			return m, nil
		}
	}

	// Handle viewport updates for mouse wheel (only when ready)
	if m.ready {
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m PRReviewModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\nPress 'q' to go back", m.err)
	}

	if len(m.pr.Comments) == 0 {
		return "No comments found on this PR.\n\nPress 'q' to go back"
	}

	// If not ready, show a simple view without viewport
	if !m.ready {
		return m.simpleView()
	}

	// Build the header
	var header strings.Builder
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86"))

	titleLine := headerStyle.Render(fmt.Sprintf("PR #%d: %s", m.pr.Number, m.pr.Title))
	// Truncate title if too long
	if lipgloss.Width(titleLine) > m.width-2 {
		title := m.pr.Title
		if len(title) > m.width-20 {
			title = title[:m.width-23] + "..."
		}
		titleLine = headerStyle.Render(fmt.Sprintf("PR #%d: %s", m.pr.Number, title))
	}
	header.WriteString(titleLine)
	header.WriteString("\n")

	acceptedCount := len(m.pr.GetAcceptedComments())
	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))
	
	// Add scroll indicators and debug info
	scrollInfo := ""
	if m.viewport.TotalLineCount() > m.viewport.Height {
		scrollPercent := int(m.viewport.ScrollPercent() * 100)
		scrollInfo = fmt.Sprintf(" | %d%%", scrollPercent)
		if m.viewport.AtTop() {
			scrollInfo += " ↓"
		} else if m.viewport.AtBottom() {
			scrollInfo += " ↑"
		} else {
			scrollInfo += " ↕"
		}
	}
	// Debug: show viewport dimensions
	// scrollInfo += fmt.Sprintf(" | H:%d/%d", m.viewport.Height, m.height)
	
	header.WriteString(statusStyle.Render(fmt.Sprintf("Comments: %d total, %d accepted | Comment %d/%d%s", 
		len(m.pr.Comments), acceptedCount, m.currentIndex+1, len(m.pr.Comments), scrollInfo)))
	header.WriteString("\n\n") // Add blank line between header and content

	// Build the footer (help text)
	var footer string
	if m.showHelp {
		helpStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
		
		helpItems := []string{
			"j/k:nav",
			"a/d:accept/deny",
			"A/D:all",
			"Enter:process",
			"q:cancel",
			"PgUp/PgDn:scroll",
			"g/G:top/bottom",
			"?:help",
		}
		footer = "\n" + helpStyle.Render(strings.Join(helpItems, " • "))
	}

	// Combine everything - header already has newlines, viewport content, then footer
	return header.String() + m.viewport.View() + footer
}

func (m *PRReviewModel) updateViewportContent() {
	var content strings.Builder
	
	for i, comment := range m.pr.Comments {
		if i > 0 {
			content.WriteString("\n")
		}
		
		// Comment box styling
		var boxStyle lipgloss.Style
		isSelected := i == m.currentIndex
		
		maxWidth := m.width - 4
		if maxWidth < 40 {
			maxWidth = 40
		}
		
		if isSelected {
			boxStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("86")).
				Padding(0, 1).
				Width(maxWidth)
		} else {
			boxStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.HiddenBorder()).
				Padding(0, 1).
				Width(maxWidth)
		}

		// Status indicator
		status := "[ ]"
		if comment.Accepted {
			status = "[✓]"
		}

		// Build header
		header := fmt.Sprintf("%s %s @%s", status, comment.Type, comment.Author)
		if comment.Path != "" {
			header += fmt.Sprintf(" • %s", comment.Path)
			if comment.Line > 0 {
				header += fmt.Sprintf(":%d", comment.Line)
			}
		}
		
		// Truncate header if too long
		if len(header) > maxWidth-4 {
			header = header[:maxWidth-7] + "..."
		}

		// Format body
		body := comment.GetFormattedBody()
		// Limit body length for non-selected items
		if !isSelected && len(body) > 150 {
			body = body[:147] + "..."
		}
		
		// Wrap text to fit within box
		lines := m.wrapText(body, maxWidth-4)
		wrappedBody := strings.Join(lines, "\n")

		// Combine header and body
		commentContent := fmt.Sprintf("%s\n\n%s",
			lipgloss.NewStyle().Bold(true).Render(header),
			wrappedBody)

		// Add selection indicator
		prefix := "  "
		if isSelected {
			prefix = "> "
		}
		
		content.WriteString(prefix + boxStyle.Render(commentContent))
		content.WriteString("\n")
	}
	
	m.viewport.SetContent(content.String())
}

func (m *PRReviewModel) wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	
	var result []string
	lines := strings.Split(text, "\n")
	
	for _, line := range lines {
		if len(line) <= width {
			result = append(result, line)
			continue
		}
		
		// Wrap long lines
		for len(line) > width {
			// Find last space before width
			lastSpace := width
			for i := width; i > 0; i-- {
				if line[i-1] == ' ' {
					lastSpace = i
					break
				}
			}
			
			// If no space found, just cut at width
			if lastSpace == width {
				result = append(result, line[:width])
				line = line[width:]
			} else {
				result = append(result, line[:lastSpace-1])
				line = line[lastSpace:]
			}
		}
		
		if len(line) > 0 {
			result = append(result, line)
		}
	}
	
	return result
}

func (m *PRReviewModel) ensureCurrentCommentVisible() {
	// This is a simple implementation - could be improved to calculate
	// exact position of current comment
	lines := strings.Split(m.viewport.View(), "\n")
	totalLines := len(lines)
	
	if totalLines == 0 {
		return
	}
	
	// Estimate position based on comment index
	estimatedPosition := float64(m.currentIndex) / float64(len(m.pr.Comments))
	targetLine := int(estimatedPosition * float64(m.viewport.TotalLineCount()))
	
	// Scroll to make the comment visible
	if targetLine < m.viewport.YOffset {
		m.viewport.SetYOffset(targetLine)
	} else if targetLine > m.viewport.YOffset+m.viewport.Height-5 {
		m.viewport.SetYOffset(targetLine - m.viewport.Height + 5)
	}
}

// simpleView renders a basic view without viewport when not ready
func (m PRReviewModel) simpleView() string {
	var b strings.Builder
	
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86"))
	
	b.WriteString(headerStyle.Render(fmt.Sprintf("PR #%d: %s", m.pr.Number, m.pr.Title)))
	b.WriteString("\n\n")
	
	acceptedCount := len(m.pr.GetAcceptedComments())
	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))
	b.WriteString(statusStyle.Render(fmt.Sprintf("Comments: %d total, %d accepted", len(m.pr.Comments), acceptedCount)))
	b.WriteString("\n\n")
	
	// Show current comment
	if len(m.pr.Comments) > 0 && m.currentIndex < len(m.pr.Comments) {
		comment := m.pr.Comments[m.currentIndex]
		
		status := "[ ]"
		if comment.Accepted {
			status = "[✓]"
		}
		
		b.WriteString(fmt.Sprintf("Comment %d/%d:\n", m.currentIndex+1, len(m.pr.Comments)))
		b.WriteString(fmt.Sprintf("%s %s @%s\n", status, comment.Type, comment.Author))
		if comment.Path != "" {
			b.WriteString(fmt.Sprintf("File: %s", comment.Path))
			if comment.Line > 0 {
				b.WriteString(fmt.Sprintf(":%d", comment.Line))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
		
		body := comment.Body
		if len(body) > 500 {
			body = body[:497] + "..."
		}
		b.WriteString(body)
		b.WriteString("\n\n")
	}
	
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))
	
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("Keys: j/k:nav • a/d:accept/deny • Enter:process • q:cancel"))
	
	return b.String()
}