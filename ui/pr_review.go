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
	}
}

func (m PRReviewModel) Init() tea.Cmd {
	return nil
}

func (m PRReviewModel) Update(msg tea.Msg) (PRReviewModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			return m, func() tea.Msg { return PRReviewCancelMsg{} }
		
		case "?":
			m.showHelp = !m.showHelp
			return m, nil
		
		case "j", "down":
			if m.currentIndex < len(m.pr.Comments)-1 {
				m.currentIndex++
			}
			return m, nil
		
		case "k", "up":
			if m.currentIndex > 0 {
				m.currentIndex--
			}
			return m, nil
		
		case "a":
			if len(m.pr.Comments) > 0 {
				m.pr.Comments[m.currentIndex].Accepted = true
			}
			return m, nil
		
		case "d":
			if len(m.pr.Comments) > 0 {
				m.pr.Comments[m.currentIndex].Accepted = false
			}
			return m, nil
		
		case "A":
			for i := range m.pr.Comments {
				m.pr.Comments[i].Accepted = true
			}
			return m, nil
		
		case "D":
			for i := range m.pr.Comments {
				m.pr.Comments[i].Accepted = false
			}
			return m, nil
		
		case "enter":
			acceptedComments := m.pr.GetAcceptedComments()
			return m, func() tea.Msg { return PRReviewCompleteMsg{AcceptedComments: acceptedComments} }
		}
	}

	return m, nil
}

func (m PRReviewModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\nPress 'q' to go back", m.err)
	}

	if len(m.pr.Comments) == 0 {
		return "No comments found on this PR.\n\nPress 'q' to go back"
	}

	var b strings.Builder

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86")).
		MarginBottom(1)

	b.WriteString(headerStyle.Render(fmt.Sprintf("PR #%d: %s", m.pr.Number, m.pr.Title)))
	b.WriteString("\n\n")

	acceptedCount := len(m.pr.GetAcceptedComments())
	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))
	b.WriteString(statusStyle.Render(fmt.Sprintf("Comments: %d total, %d accepted", len(m.pr.Comments), acceptedCount)))
	b.WriteString("\n\n")

	for i, comment := range m.pr.Comments {
		var commentStyle lipgloss.Style
		prefix := "  "
		
		if i == m.currentIndex {
			commentStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("86")).
				Padding(1).
				Width(m.width - 4)
			prefix = "> "
		} else {
			commentStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.HiddenBorder()).
				Padding(1).
				Width(m.width - 4)
		}

		status := "[ ]"
		if comment.Accepted {
			status = "[✓]"
		}

		header := fmt.Sprintf("%s %s @%s", status, comment.Type, comment.Author)
		if comment.Path != "" {
			header += fmt.Sprintf(" • %s:%d", comment.Path, comment.Line)
		}
		header += fmt.Sprintf(" • %s", comment.CreatedAt.Format("Jan 2, 2006"))

		content := comment.GetFormattedBody()
		if len(content) > 200 && i != m.currentIndex {
			content = content[:197] + "..."
		}

		commentBlock := fmt.Sprintf("%s\n\n%s", 
			lipgloss.NewStyle().Bold(true).Render(header),
			content)

		b.WriteString(fmt.Sprintf("%s%s\n\n", prefix, commentStyle.Render(commentBlock)))
	}

	if m.showHelp {
		helpStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginTop(2)
		
		helpText := []string{
			"Navigation: j/k or ↓/↑",
			"Accept: a | Deny: d",
			"Accept All: A | Deny All: D", 
			"Process: Enter | Cancel: q/Esc",
			"Toggle Help: ?",
		}
		b.WriteString(helpStyle.Render(strings.Join(helpText, " • ")))
	}

	return b.String()
}