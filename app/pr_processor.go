package app

import (
	"claude-squad/session/git"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type processCommentMsg struct {
	comment git.PRComment
	index   int
	total   int
}

type commentProcessedMsg struct {
	comment git.PRComment
	success bool
	err     error
}

type allCommentsProcessedMsg struct{}

func (m *home) processAcceptedComments(comments []git.PRComment) tea.Cmd {
	return func() tea.Msg {
		// Create a text overlay to show progress
		progressText := fmt.Sprintf("Processing %d PR comments...\n\n", len(comments))
		m.textOverlay = overlay.NewTextOverlay("PR Comment Processing", progressText)
		m.state = stateHelp // Reuse help state for overlay display
		
		// Process each comment sequentially
		for i, comment := range comments {
			// Update progress
			progressText += fmt.Sprintf("[%d/%d] Processing comment from @%s...\n", i+1, len(comments), comment.Author)
			m.textOverlay.SetContent(progressText)
			
			// Send comment to Claude for processing
			if err := m.sendCommentToClaude(comment); err != nil {
				progressText += fmt.Sprintf("  ❌ Error: %v\n", err)
				m.textOverlay.SetContent(progressText)
			} else {
				progressText += fmt.Sprintf("  ✓ Sent to Claude\n")
				m.textOverlay.SetContent(progressText)
			}
			
			// Small delay between comments
			time.Sleep(500 * time.Millisecond)
		}
		
		progressText += "\nAll comments processed. Press any key to continue."
		m.textOverlay.SetContent(progressText)
		
		return allCommentsProcessedMsg{}
	}
}

func (m *home) sendCommentToClaude(comment git.PRComment) error {
	selected := m.list.GetSelectedInstance()
	if selected == nil {
		return fmt.Errorf("no instance selected")
	}
	
	// Format the comment as a prompt for Claude
	prompt := m.formatCommentAsPrompt(comment)
	
	// Send prompt to the instance
	return selected.SendPrompt(prompt)
}

func (m *home) formatCommentAsPrompt(comment git.PRComment) string {
	var prompt strings.Builder
	
	prompt.WriteString(fmt.Sprintf("PR Review Comment from @%s:\n\n", comment.Author))
	
	if comment.Path != "" {
		prompt.WriteString(fmt.Sprintf("File: %s", comment.Path))
		if comment.Line > 0 {
			prompt.WriteString(fmt.Sprintf(" (line %d)", comment.Line))
		}
		prompt.WriteString("\n\n")
	}
	
	prompt.WriteString("Comment:\n")
	prompt.WriteString(comment.Body)
	prompt.WriteString("\n\n")
	
	prompt.WriteString("Please address this review comment by making the necessary changes to the code.")
	
	return prompt.String()
}