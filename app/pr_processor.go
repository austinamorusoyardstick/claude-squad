package app

import (
	"claude-squad/session"
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
	// Store comments for sequential processing
	m.pendingPRComments = comments
	m.currentPRCommentIndex = 0
	
	// Show initial progress
	m.updatePRProcessingOverlay()
	
	// Start processing the first comment
	return m.processNextPRComment()
}

func (m *home) processCommentsSequentially(comments []git.PRComment) tea.Cmd {
	return func() tea.Msg {
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return fmt.Errorf("no instance selected")
		}
		
		// Check if instance is ready
		if selected.Status != session.Ready && selected.Status != session.Running {
			return fmt.Errorf("instance is not ready to receive prompts (status: %v)", selected.Status)
		}
		
		// Process each comment
		for i, comment := range comments {
			prompt := m.formatCommentAsPrompt(comment)
			if err := selected.SendPromptToAI(prompt); err != nil {
				return fmt.Errorf("failed to send comment %d to Claude: %w", i+1, err)
			}
			// Longer delay between comments to give Claude time to process
			time.Sleep(2 * time.Second)
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
	
	prompt.WriteString("=== PR REVIEW COMMENT ===\n\n")
	prompt.WriteString(fmt.Sprintf("Author: @%s\n", comment.Author))
	prompt.WriteString(fmt.Sprintf("Type: %s comment\n", comment.Type))
	
	if comment.Path != "" {
		prompt.WriteString(fmt.Sprintf("File: %s", comment.Path))
		if comment.Line > 0 {
			prompt.WriteString(fmt.Sprintf(" (line %d)", comment.Line))
		}
		prompt.WriteString("\n")
	}
	
	prompt.WriteString("\nComment:\n")
	prompt.WriteString("---\n")
	prompt.WriteString(comment.Body)
	prompt.WriteString("\n---\n\n")
	
	prompt.WriteString("Please analyze this pull request review comment and make the necessary changes to address the feedback. ")
	prompt.WriteString("If the comment is asking a question, provide a clear answer. ")
	prompt.WriteString("If it's suggesting a change, implement it. ")
	prompt.WriteString("If you need clarification, explain what's unclear.")
	
	return prompt.String()
}