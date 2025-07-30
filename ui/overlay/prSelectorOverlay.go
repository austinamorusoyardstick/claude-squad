package overlay

import (
	"claude-squad/keys"
	"claude-squad/session/git"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type PRSelectorOverlay struct {
	prs          []*git.PullRequest
	selectedPRs  map[int]bool
	cursor       int
	workingDir   string
	onComplete   func([]*git.PullRequest)
	fetchingPRs  bool
	errorMessage string
	width        int
	height       int
	title        string
	borderColor  string
}

func NewPRSelectorOverlay(workingDir string, onComplete func([]*git.PullRequest)) *PRSelectorOverlay {
	return &PRSelectorOverlay{
		title:       "Select PRs to Merge",
		borderColor: "#04B575",
		prs:         []*git.PullRequest{},
		selectedPRs: make(map[int]bool),
		workingDir:  workingDir,
		onComplete:  onComplete,
		fetchingPRs: true,
		width:       80,
		height:      20,
	}
}

func (o *PRSelectorOverlay) Init() tea.Cmd {
	return o.fetchPRs()
}

func (o *PRSelectorOverlay) fetchPRs() tea.Cmd {
	return func() tea.Msg {
		prs, err := git.ListOpenPRs(o.workingDir)
		if err != nil {
			return prsFetchedMsg{err: err}
		}
		return prsFetchedMsg{prs: prs}
	}
}

type prsFetchedMsg struct {
	prs []*git.PullRequest
	err error
}

func (o *PRSelectorOverlay) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		o.width = msg.Width
		o.height = msg.Height
		return o, nil

	case prsFetchedMsg:
		o.fetchingPRs = false
		if msg.err != nil {
			o.errorMessage = fmt.Sprintf("Error fetching PRs: %v", msg.err)
			return o, nil
		}
		o.prs = msg.prs
		if len(o.prs) == 0 {
			o.errorMessage = "No open PRs found"
		}
		return o, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.GlobalkeyBindings[keys.KeyQuit]):
			return nil, nil

		case key.Matches(msg, keys.GlobalkeyBindings[keys.KeyUp]):
			if o.cursor > 0 {
				o.cursor--
			}

		case key.Matches(msg, keys.GlobalkeyBindings[keys.KeyDown]):
			if o.cursor < len(o.prs)-1 {
				o.cursor++
			}

		case msg.String() == " ":
			if o.cursor < len(o.prs) {
				pr := o.prs[o.cursor]
				o.selectedPRs[pr.Number] = !o.selectedPRs[pr.Number]
			}

		case key.Matches(msg, keys.GlobalkeyBindings[keys.KeyEnter]):
			// Collect selected PRs
			selected := []*git.PullRequest{}
			for _, pr := range o.prs {
				if o.selectedPRs[pr.Number] {
					selected = append(selected, pr)
				}
			}
			if len(selected) > 0 && o.onComplete != nil {
				o.onComplete(selected)
			}
			return nil, nil
		}
	}

	return o, nil
}

func (o *PRSelectorOverlay) View() string {
	if o.width == 0 || o.height == 0 {
		return ""
	}

	var content strings.Builder

	if o.fetchingPRs {
		content.WriteString("Fetching open PRs...")
	} else if o.errorMessage != "" {
		content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(o.errorMessage))
		content.WriteString("\n\nPress q to close")
	} else if len(o.prs) == 0 {
		content.WriteString("No open PRs found")
		content.WriteString("\n\nPress q to close")
	} else {
		content.WriteString("Select PRs to merge (space to select, enter to confirm):\n\n")

		for i, pr := range o.prs {
			checkbox := "[ ]"
			if o.selectedPRs[pr.Number] {
				checkbox = "[x]"
			}

			cursor := "  "
			if i == o.cursor {
				cursor = "> "
			}

			line := fmt.Sprintf("%s%s #%d: %s (%s -> %s)",
				cursor, checkbox, pr.Number, pr.Title, pr.HeadRef, pr.BaseRef)

			if i == o.cursor {
				content.WriteString(lipgloss.NewStyle().Bold(true).Render(line))
			} else {
				content.WriteString(line)
			}
			content.WriteString("\n")
		}

		selectedCount := len(o.selectedPRs)
		for _, selected := range o.selectedPRs {
			if !selected {
				selectedCount--
			}
		}

		content.WriteString(fmt.Sprintf("\n%d PR(s) selected", selectedCount))
		content.WriteString("\n\nSpace: Toggle selection | Enter: Confirm | q: Cancel")
	}

	// Add padding and render with border
	contentStr := content.String()
	lines := strings.Split(contentStr, "\n")
	maxWidth := 0
	for _, line := range lines {
		if len(line) > maxWidth {
			maxWidth = len(line)
		}
	}

	// Ensure minimum width
	if maxWidth < 60 {
		maxWidth = 60
	}

	// Pad each line to the same width
	paddedLines := make([]string, len(lines))
	for i, line := range lines {
		paddedLines[i] = line + strings.Repeat(" ", maxWidth-len(line))
	}

	paddedContent := strings.Join(paddedLines, "\n")

	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(o.borderColor)).
		Padding(1, 2)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(o.borderColor))

	// Combine title and content
	finalContent := titleStyle.Render(o.title) + "\n\n" + paddedContent

	// Apply border and center
	bordered := border.Render(finalContent)
	return lipgloss.Place(o.width, o.height, lipgloss.Center, lipgloss.Center, bordered)
}
