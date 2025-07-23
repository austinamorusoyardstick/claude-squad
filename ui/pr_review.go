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
	filterEnabled bool
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
		pr:            pr,
		currentIndex:  0,
		showHelp:      true,
		filterEnabled: true,  // Default to filter enabled
		ready:         false,
		width:         80,   // Default width
		height:        24,   // Default height
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
		
		// Header: title (1) + status (1) + blank line after status (1) = 3
		headerHeight := 3
		// Footer: blank line before help (1) + help text (1) = 2
		footerHeight := 2
		if !m.showHelp {
			footerHeight = 0
		}
		
		// Minimal safety margin since we have padding in the content
		safetyMargin := 0
		
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
				headerHeight := 3
				footerHeight := 2
				if !m.showHelp {
					footerHeight = 0
				}
				safetyMargin := 0
				m.viewport.Height = m.height - headerHeight - footerHeight - safetyMargin
				m.updateViewportContent()
			}
			return m, nil
		
		case "f":
			m.filterEnabled = !m.filterEnabled
			// Reset current index to ensure it's valid
			m.currentIndex = 0
			if m.ready {
				m.updateViewportContent()
				m.viewport.SetYOffset(0) // Reset scroll position
			}
			return m, nil
		
		case "j", "down":
			if m.currentIndex < len(m.getActiveComments())-1 {
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
			comments := m.getActiveComments()
			if len(comments) > 0 {
				comments[m.currentIndex].Accepted = true
				if m.ready {
					m.updateViewportContent()
				}
			}
			return m, nil
		
		case "d":
			comments := m.getActiveComments()
			if len(comments) > 0 {
				comments[m.currentIndex].Accepted = false
				if m.ready {
					m.updateViewportContent()
				}
			}
			return m, nil
		
		case "A":
			comments := m.getActiveComments()
			for i := range comments {
				comments[i].Accepted = true
			}
			if m.ready {
				m.updateViewportContent()
			}
			return m, nil
		
		case "D":
			comments := m.getActiveComments()
			for i := range comments {
				comments[i].Accepted = false
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
			if len(m.getActiveComments()) > 0 {
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
			if len(m.getActiveComments()) > 0 {
				m.currentIndex = len(m.getActiveComments()) - 1
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

// getActiveComments returns the comments based on filter state
func (m *PRReviewModel) getActiveComments() []git.PRComment {
	if m.filterEnabled {
		return m.pr.Comments
	}
	return m.pr.AllComments
}

func (m PRReviewModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\nPress 'q' to go back", m.err)
	}

	comments := m.getActiveComments()
	if len(comments) == 0 {
		if m.filterEnabled && len(m.pr.AllComments) > 0 {
			return "No active comments found on this PR (all are outdated/resolved).\n\nPress 'f' to show all comments\nPress 'q' to go back"
		}
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
	
	// Show filter status
	filterStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("28")).
		Italic(true)
	if m.filterEnabled {
		header.WriteString(filterStyle.Render("(Filter: ON - hiding outdated/resolved)"))
	} else {
		header.WriteString(filterStyle.Render("(Filter: OFF - showing all comments)"))
	}
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
	
	comments := m.getActiveComments()
	if m.filterEnabled {
		total, reviews, reviewComments, issueComments, _, _ := m.pr.GetCommentStats()
		header.WriteString(statusStyle.Render(fmt.Sprintf("Comments: %d (%dR %dRC %dG), %d accepted | %d/%d%s", 
			total, reviews, reviewComments, issueComments, acceptedCount, m.currentIndex+1, len(comments), scrollInfo)))
	} else {
		// Count stats from all comments
		var total, reviews, reviewComments, issueComments, outdated, resolved int
		for _, comment := range m.pr.AllComments {
			total++
			if comment.IsOutdated {
				outdated++
			}
			if comment.IsResolved {
				resolved++
			}
			switch comment.Type {
			case "review":
				reviews++
			case "review_comment":
				reviewComments++
			case "issue_comment":
				issueComments++
			}
		}
		header.WriteString(statusStyle.Render(fmt.Sprintf("All Comments: %d (%dR %dRC %dG, %d outdated, %d resolved), %d accepted | %d/%d%s", 
			total, reviews, reviewComments, issueComments, outdated, resolved, acceptedCount, m.currentIndex+1, len(comments), scrollInfo)))
	}
	header.WriteString("\n") // Single newline after status

	// Build the footer (help text)
	var footer string
	if m.showHelp {
		helpStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
		
		helpItems := []string{
			"j/k:nav",
			"a/d:accept/deny",
			"A/D:all",
			"f:toggle filter",
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
	
	comments := m.getActiveComments()
	for i, comment := range comments {
		if i > 0 {
			content.WriteString("\n\n") // Add consistent spacing between comments
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
		
		// Add visual indicator if comment was originally resolved but still showing (shouldn't happen)
		if comment.IsResolved {
			status += " (resolved)"
		}

		// Build header with better type display
		typeDisplay := comment.Type
		switch comment.Type {
		case "review":
			typeDisplay = "PR Review"
		case "review_comment":
			typeDisplay = "Review Comment"
		case "issue_comment":
			typeDisplay = "General Comment"
		}
		header := fmt.Sprintf("%s %s by @%s", status, typeDisplay, comment.Author)
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
	}
	
	// Add substantial padding after the last comment to ensure it's fully visible when scrolled to bottom
	// This padding should be at least the height of a typical comment box
	content.WriteString("\n\n\n\n\n\n\n\n")
	
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
	comments := m.getActiveComments()
	estimatedPosition := float64(m.currentIndex) / float64(len(comments))
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
	b.WriteString("\n")
	
	// Show filter status
	filterStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("28")).
		Italic(true)
	if m.filterEnabled {
		b.WriteString(filterStyle.Render("(Filter: ON - hiding outdated/resolved)"))
	} else {
		b.WriteString(filterStyle.Render("(Filter: OFF - showing all comments)"))
	}
	b.WriteString("\n\n")
	
	acceptedCount := len(m.pr.GetAcceptedComments())
	
	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))
	
	comments := m.getActiveComments()
	
	if m.filterEnabled {
		total, reviews, reviewComments, issueComments, _, _ := m.pr.GetCommentStats()
		
		// Show comment breakdown
		b.WriteString(statusStyle.Render(fmt.Sprintf("Comments: %d (%d reviews, %d review comments, %d general)", 
			total, reviews, reviewComments, issueComments)))
		b.WriteString("\n")
		
		// Count filtered comments
		allTotal := len(m.pr.AllComments)
		filtered := allTotal - total
		if filtered > 0 {
			b.WriteString(statusStyle.Render(fmt.Sprintf("Filtered out: %d outdated/resolved", filtered)))
			b.WriteString("\n")
		}
	} else {
		// Count stats from all comments
		var total, reviews, reviewComments, issueComments, outdated, resolved int
		for _, comment := range m.pr.AllComments {
			total++
			if comment.IsOutdated {
				outdated++
			}
			if comment.IsResolved {
				resolved++
			}
			switch comment.Type {
			case "review":
				reviews++
			case "review_comment":
				reviewComments++
			case "issue_comment":
				issueComments++
			}
		}
		
		b.WriteString(statusStyle.Render(fmt.Sprintf("All Comments: %d (%d reviews, %d review comments, %d general)", 
			total, reviews, reviewComments, issueComments)))
		b.WriteString("\n")
		
		if outdated > 0 || resolved > 0 {
			b.WriteString(statusStyle.Render(fmt.Sprintf("Including: %d outdated, %d resolved", outdated, resolved)))
			b.WriteString("\n")
		}
	}
	
	b.WriteString(statusStyle.Render(fmt.Sprintf("Accepted: %d", acceptedCount)))
	b.WriteString("\n\n")
	
	// Show current comment
	if len(comments) > 0 && m.currentIndex < len(comments) {
		comment := comments[m.currentIndex]
		
		status := "[ ]"
		if comment.Accepted {
			status = "[✓]"
		}
		
		// Add visual indicator if comment was originally resolved but still showing (shouldn't happen)
		if comment.IsResolved {
			status += " (resolved)"
		}
		
		b.WriteString(fmt.Sprintf("Comment %d/%d:\n", m.currentIndex+1, len(comments)))
		
		// Format comment type with better descriptions
		typeDisplay := comment.Type
		switch comment.Type {
		case "review":
			typeDisplay = "PR Review"
		case "review_comment":
			typeDisplay = "Review Comment"
		case "issue_comment":
			typeDisplay = "General Comment"
		}
		
		b.WriteString(fmt.Sprintf("%s %s by @%s\n", status, typeDisplay, comment.Author))
		
		// Show file location if available
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
	b.WriteString(helpStyle.Render("Keys: j/k:nav • a/d:accept/deny • f:toggle filter • Enter:process • q:cancel"))
	
	return b.String()
}