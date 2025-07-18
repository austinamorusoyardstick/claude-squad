package app

import (
	"claude-squad/session/git"
	"claude-squad/ui/overlay"
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
	// First show the processing overlay
	progressText := fmt.Sprintf("Processing %d PR comments...\n\n", len(comments))
	for i, comment := range comments {
		progressText += fmt.Sprintf("%d. Comment from @%s", i+1, comment.Author)
		if comment.Path != "" {
			progressText += fmt.Sprintf(" on %s", comment.Path)
			if comment.Line > 0 {
				progressText += fmt.Sprintf(":%d", comment.Line)
			}
		}
		progressText += "\n"
	}
	progressText += "\nSending to Claude for processing..."
	
	m.textOverlay = overlay.NewTextOverlay(progressText)
	m.state = stateHelp
	
	// Return a command that processes comments
	return m.processCommentsSequentially(comments)
}

func (m *home) processCommentsSequentially(comments []git.PRComment) tea.Cmd {
	return func() tea.Msg {
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return fmt.Errorf("no instance selected")
		}
		
		// Process each comment
		for _, comment := range comments {
			prompt := m.formatCommentAsPrompt(comment)
			if err := selected.SendPromptToAI(prompt); err != nil {
				return fmt.Errorf("failed to send comment to Claude: %w", err)
			}
			// Small delay between comments to avoid overwhelming Claude
			time.Sleep(1 * time.Second)
		}
		
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