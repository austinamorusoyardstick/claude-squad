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
	
	// Bookmark mode settings
	bookmarkMode    bool
	bookmarks       []string // List of bookmark commit SHAs
	currentBookmark int      // Index in bookmarks slice (-1 for "current changes")
	worktree        *git.GitWorktree // Reference to git worktree for bookmark navigation

	width  int
	height int
}

// NewGitStatusOverlay creates a new git status overlay
func NewGitStatusOverlay(branchName string, files []git.GitFileStatus) *GitStatusOverlay {
	return &GitStatusOverlay{
		Dismissed:    false,
		files:        files,
		branchName:   branchName,
		bookmarkMode: false,
		width:        80,
		height:       20,
	}
}

// NewGitStatusOverlayBookmarkMode creates a new git status overlay in bookmark mode
func NewGitStatusOverlayBookmarkMode(branchName string, worktree *git.GitWorktree) (*GitStatusOverlay, error) {
	// Get all bookmarks
	bookmarks, err := worktree.GetAllBookmarkCommits()
	if err != nil {
		return nil, fmt.Errorf("failed to get bookmarks: %w", err)
	}

	if len(bookmarks) == 0 {
		return nil, fmt.Errorf("no bookmarks found in this branch")
	}

	// Check if there are changes after the most recent bookmark
	var currentIndex int
	lastBookmark := bookmarks[len(bookmarks)-1]
	currentChanges, err := worktree.GetChangedFilesSinceCommit(lastBookmark)
	if err == nil && len(currentChanges) > 0 {
		// Start with current changes (-1 means "current changes after last bookmark")
		currentIndex = -1
	} else {
		// No current changes, start with the most recent bookmark
		currentIndex = len(bookmarks) - 1
	}
	
	overlay := &GitStatusOverlay{
		Dismissed:       false,
		branchName:      branchName,
		bookmarkMode:    true,
		bookmarks:       bookmarks,
		currentBookmark: currentIndex,
		worktree:        worktree,
		width:           80,
		height:          20,
	}

	// Load files for the current bookmark
	if err := overlay.loadBookmarkFiles(); err != nil {
		return nil, fmt.Errorf("failed to load bookmark files: %w", err)
	}

	return overlay, nil
}

// HandleKeyPress processes a key press and updates the state
// Returns true if the overlay should be closed
func (g *GitStatusOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	// In bookmark mode, handle navigation keys
	if g.bookmarkMode {
		switch msg.String() {
		case "left":
			return g.navigateBookmark(-1)
		case "right":
			return g.navigateBookmark(1)
		case "esc", "q":
			g.Dismissed = true
			if g.OnDismiss != nil {
				g.OnDismiss()
			}
			return true
		default:
			// Any other key closes the overlay
			g.Dismissed = true
			if g.OnDismiss != nil {
				g.OnDismiss()
			}
			return true
		}
	}
	
	// In normal mode, close on any key
	g.Dismissed = true
	// Call the OnDismiss callback if it exists
	if g.OnDismiss != nil {
		g.OnDismiss()
	}
	return true
}

// navigateBookmark moves to the next/previous bookmark
// Returns false to indicate the overlay should stay open
func (g *GitStatusOverlay) navigateBookmark(direction int) bool {
	if !g.bookmarkMode || len(g.bookmarks) == 0 {
		return false
	}

	newIndex := g.currentBookmark + direction
	
	// Handle bounds: -1 is "current changes", 0 to len-1 are bookmarks
	maxIndex := len(g.bookmarks) - 1
	minIndex := -1
	
	// Check if current changes exist to determine if -1 is valid
	if minIndex == -1 && len(g.bookmarks) > 0 {
		lastBookmark := g.bookmarks[len(g.bookmarks)-1]
		currentChanges, err := g.worktree.GetChangedFilesSinceCommit(lastBookmark)
		if err != nil || len(currentChanges) == 0 {
			minIndex = 0 // No current changes, so minimum is first bookmark
		}
	}
	
	if newIndex < minIndex || newIndex > maxIndex {
		// Don't navigate beyond bounds
		return false
	}

	g.currentBookmark = newIndex
	g.cachedContent = "" // Clear cache to force re-render
	
	// Load files for the new position
	if err := g.loadBookmarkFiles(); err != nil {
		// On error, just stay at current position
		return false
	}

	return false // Keep overlay open
}

// loadBookmarkFiles loads the files changed for the current bookmark
func (g *GitStatusOverlay) loadBookmarkFiles() error {
	if !g.bookmarkMode || g.currentBookmark < 0 || g.currentBookmark >= len(g.bookmarks) {
		return fmt.Errorf("invalid bookmark index")
	}

	currentCommit := g.bookmarks[g.currentBookmark]
	var fromCommit string
	
	// If this is not the first bookmark, get changes since the previous one
	if g.currentBookmark > 0 {
		fromCommit = g.bookmarks[g.currentBookmark-1]
	}

	files, err := g.worktree.GetChangedFilesBetweenCommits(fromCommit, currentCommit)
	if err != nil {
		return fmt.Errorf("failed to get changed files between commits: %w", err)
	}

	g.files = files
	return nil
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
	var title string
	if g.bookmarkMode {
		// Get bookmark commit message
		currentCommit := g.bookmarks[g.currentBookmark]
		commitMsg, err := g.worktree.GetCommitMessage(currentCommit)
		if err != nil {
			commitMsg = "Unknown bookmark"
		}
		
		// Extract just the bookmark message (remove [BOOKMARK] prefix if present)
		bookmarkTitle := commitMsg
		if strings.HasPrefix(commitMsg, "[BOOKMARK] ") {
			bookmarkTitle = strings.TrimPrefix(commitMsg, "[BOOKMARK] ")
		}
		
		title = fmt.Sprintf("Bookmark %d/%d - %s", g.currentBookmark+1, len(g.bookmarks), bookmarkTitle)
		content.WriteString(lipgloss.NewStyle().Bold(true).Render(title))
		content.WriteString("\n")
		content.WriteString(lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf("Branch: %s | SHA: %s", g.branchName, currentCommit[:8])))
		content.WriteString("\n\n")
	} else {
		title = fmt.Sprintf("Git Status - Branch: %s", g.branchName)
		content.WriteString(lipgloss.NewStyle().Bold(true).Render(title))
		content.WriteString("\n\n")
	}

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
		
		// Display files grouped by status in a consistent order
		statusOrder := []string{"A", "M", "D", "R", "C"} // Added, Modified, Deleted, Renamed, Copied
		for _, status := range statusOrder {
			files, exists := statusGroups[status]
			if !exists {
				continue
			}
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

		// Handle any remaining statuses not in the predefined order
		for status, files := range statusGroups {
			found := false
			for _, s := range statusOrder {
				if s == status {
					found = true
					break
				}
			}
			if found {
				continue
			}

			statusName := statusNames[status]
			if statusName == "" {
				statusName = status
			}
			
			// Use default cyan color for unknown statuses
			statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
			
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
	if g.bookmarkMode {
		content.WriteString(lipgloss.NewStyle().Faint(true).Render("← → Navigate bookmarks | Any other key to close"))
	} else {
		content.WriteString(lipgloss.NewStyle().Faint(true).Render("Press any key to close"))
	}

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