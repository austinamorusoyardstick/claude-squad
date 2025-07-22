package overlay

import (
	"claude-squad/session/git"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// GitStatusOverlay represents a git status overlay showing changed files
type GitStatusOverlay struct {
	// Whether the overlay has been dismissed
	Dismissed bool
	// Callback function to be called when the overlay is dismissed
	OnDismiss func()
	// Files that have changed in this branch
	files []git.GitFileStatus
	// Branch name
	branchName string
	// Cached rendered content to prevent re-rendering
	cachedContent string

	width  int
	height int
}

// NewGitStatusOverlay creates a new git status overlay
func NewGitStatusOverlay(branchName string, files []git.GitFileStatus) *GitStatusOverlay {
	return &GitStatusOverlay{
		Dismissed:  false,
		files:      files,
		branchName: branchName,
		width:      80,
		height:     20,
	}
}

// HandleKeyPress processes a key press and updates the state
// Returns true if the overlay should be closed
func (g *GitStatusOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	// Close on any key
	g.Dismissed = true
	// Call the OnDismiss callback if it exists
	if g.OnDismiss != nil {
		g.OnDismiss()
	}
	return true
}

// Render renders the git status overlay
func (g *GitStatusOverlay) Render() string {
	// Return cached content if already rendered
	if g.cachedContent != "" {
		return g.cachedContent
	}

	// Create the content
	var content strings.Builder
	
	// Title
	title := fmt.Sprintf("Git Status - Branch: %s", g.branchName)
	content.WriteString(lipgloss.NewStyle().Bold(true).Render(title))
	content.WriteString("\n\n")

	if len(g.files) == 0 {
		content.WriteString("No files changed in this branch.")
	} else {
		// Group files by status
		statusGroups := make(map[string][]string)
		statusNames := map[string]string{
			"M": "Modified",
			"A": "Added", 
			"D": "Deleted",
			"R": "Renamed",
			"C": "Copied",
		}
		
		for _, file := range g.files {
			status := file.Status
			if len(status) > 1 {
				status = string(status[0]) // Take first character for complex statuses
			}
			statusGroups[status] = append(statusGroups[status], file.Path)
		}
		
		// Display files grouped by status
		for status, files := range statusGroups {
			statusName := statusNames[status]
			if statusName == "" {
				statusName = status
			}
			
			// Color code the status
			var statusStyle lipgloss.Style
			switch status {
			case "M":
				statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // Yellow
			case "A":
				statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // Green
			case "D":
				statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))  // Red
			default:
				statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("14")) // Cyan
			}
			
			content.WriteString(statusStyle.Render(fmt.Sprintf("● %s (%s):", statusName, status)))
			content.WriteString("\n")
			
			for _, file := range files {
				content.WriteString(fmt.Sprintf("  %s", file))
				content.WriteString("\n")
			}
			content.WriteString("\n")
		}
		
		// Summary
		content.WriteString(fmt.Sprintf("Total: %d files changed", len(g.files)))
	}
	
	content.WriteString("\n\n")
	content.WriteString(lipgloss.NewStyle().Faint(true).Render("Press any key to close"))

	// Create styles
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(g.width).
		Height(g.height)

	// Apply the border style and cache the result
	g.cachedContent = style.Render(content.String())
	return g.cachedContent
}

// SetSize sets the size of the overlay
func (g *GitStatusOverlay) SetSize(width, height int) {
	g.width = width
	g.height = height
}