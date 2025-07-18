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
		
		headerHeight := 4 // Title + status line
		footerHeight := 3 // Help text
		if !m.showHelp {
			footerHeight = 0
		}
		
		if !m.ready {
			m.viewport = viewport.New(m.width, m.height-headerHeight-footerHeight)
			m.viewport.HighPerformanceRendering = false
			m.ready = true
		} else {
			m.viewport.Width = m.width
			m.viewport.Height = m.height - headerHeight - footerHeight
		}
		
		m.updateViewportContent()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			return m, func() tea.Msg { return PRReviewCancelMsg{} }
		
		case "?":
			m.showHelp = !m.showHelp
			// Adjust viewport height when help is toggled
			headerHeight := 4
			footerHeight := 3
			if !m.showHelp {
				footerHeight = 0
			}
			m.viewport.Height = m.height - headerHeight - footerHeight
			m.updateViewportContent()
			return m, nil
		
		case "j", "down":
			if m.currentIndex < len(m.pr.Comments)-1 {
				m.currentIndex++
				m.updateViewportContent()
				m.ensureCurrentCommentVisible()
			}
			return m, nil
		
		case "k", "up":
			if m.currentIndex > 0 {
				m.currentIndex--
				m.updateViewportContent()
				m.ensureCurrentCommentVisible()
			}
			return m, nil
		
		case "a":
			if len(m.pr.Comments) > 0 {
				m.pr.Comments[m.currentIndex].Accepted = true
				m.updateViewportContent()
			}
			return m, nil
		
		case "d":
			if len(m.pr.Comments) > 0 {
				m.pr.Comments[m.currentIndex].Accepted = false
				m.updateViewportContent()
			}
			return m, nil
		
		case "A":
			for i := range m.pr.Comments {
				m.pr.Comments[i].Accepted = true
			}
			m.updateViewportContent()
			return m, nil
		
		case "D":
			for i := range m.pr.Comments {
				m.pr.Comments[i].Accepted = false
			}
			m.updateViewportContent()
			return m, nil
		
		case "enter":
			acceptedComments := m.pr.GetAcceptedComments()
			return m, func() tea.Msg { return PRReviewCompleteMsg{AcceptedComments: acceptedComments} }
		
		// Additional viewport controls
		case "pgup", "shift+up":
			m.viewport.ViewUp()
			return m, nil
		
		case "pgdown", "shift+down":
			m.viewport.ViewDown()
			return m, nil
		
		case "home", "g":
			m.viewport.GotoTop()
			if len(m.pr.Comments) > 0 {
				m.currentIndex = 0
				m.updateViewportContent()
			}
			return m, nil
		
		case "end", "G":
			m.viewport.GotoBottom()
			if len(m.pr.Comments) > 0 {
				m.currentIndex = len(m.pr.Comments) - 1
				m.updateViewportContent()
			}
			return m, nil
		}
	}

	// Handle viewport updates for mouse wheel
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m PRReviewModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\nPress 'q' to go back", m.err)
	}

	if len(m.pr.Comments) == 0 {
		return "No comments found on this PR.\n\nPress 'q' to go back"
	}

	if !m.ready {
		return "Loading..."
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
	header.WriteString(statusStyle.Render(fmt.Sprintf("Comments: %d total, %d accepted | Comment %d/%d", 
		len(m.pr.Comments), acceptedCount, m.currentIndex+1, len(m.pr.Comments))))
	header.WriteString("\n")

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
			"?:help",
		}
		footer = "\n" + helpStyle.Render(strings.Join(helpItems, " • "))
	}

	// Combine everything
	return fmt.Sprintf("%s\n%s%s", header.String(), m.viewport.View(), footer)
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