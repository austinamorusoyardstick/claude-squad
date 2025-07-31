package app

import (
	"claude-squad/log"
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

func (m *home) processAcceptedComments(comments []*git.PRComment) tea.Cmd {
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

	// No need to switch tabs - SendPromptToAI sends directly to AI pane

	// Return a command that processes comments
	return m.processCommentsSequentially(comments)
}

func (m *home) processCommentsSequentially(comments []*git.PRComment) tea.Cmd {
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
			prompt := m.formatCommentAsPrompt(comment, i+1, len(comments))

			// Skip empty prompts (e.g., split comments with no accepted pieces)
			if prompt == "" {
				log.DebugLog.Printf("Skipping comment %d (empty prompt)", i+1)
				continue
			}

			// Debug: log the prompt being sent
			log.InfoLog.Printf("Sending PR comment %d/%d to Claude", i+1, len(comments))
			promptPreview := prompt
			if len(promptPreview) > 100 {
				promptPreview = promptPreview[:100] + "..."
			}
			log.InfoLog.Printf("Prompt preview: %s", promptPreview)

			// Send the comment to Claude
			if err := selected.SendPromptToAI(prompt); err != nil {
				log.ErrorLog.Printf("Failed to send comment %d to Claude: %v", i+1, err)
				return fmt.Errorf("failed to send comment %d to Claude: %w", i+1, err)
			}

			log.InfoLog.Printf("Successfully sent comment %d to Claude", i+1)

			// Delay between comments to give Claude time to process
			if i < len(comments)-1 {
				time.Sleep(2 * time.Second)
			}
		}

		return allCommentsProcessedMsg{}
	}
}

func (m *home) sendCommentToClaude(comment *git.PRComment) error {
	selected := m.list.GetSelectedInstance()
	if selected == nil {
		return fmt.Errorf("no instance selected")
	}

	// Format the comment as a prompt for Claude
	prompt := m.formatCommentAsPrompt(comment, 1, 1)

	// Send prompt to the instance
	return selected.SendPrompt(prompt)
}

func (m *home) formatCommentAsPrompt(comment *git.PRComment, index int, total int) string {
	var prompt strings.Builder

	// Add processing header with comment number
	prompt.WriteString(fmt.Sprintf("Processing PR comment %d of %d from @%s", index, total, comment.Author))
	if comment.IsSplit {
		acceptedCount := len(comment.GetAcceptedPieces())
		prompt.WriteString(fmt.Sprintf(" (%d pieces selected)", acceptedCount))
	}
	prompt.WriteString("\n\n")

	// Format header based on comment type
	switch comment.Type {
	case "review":
		prompt.WriteString("=== PR REVIEW ===\n\n")
	case "review_comment":
		prompt.WriteString("=== PR REVIEW COMMENT ===\n\n")
	case "issue_comment":
		prompt.WriteString("=== PR GENERAL COMMENT ===\n\n")
	default:
		prompt.WriteString("=== PR COMMENT ===\n\n")
	}

	prompt.WriteString(fmt.Sprintf("Author: @%s\n", comment.Author))

	// Better type descriptions
	typeDisplay := comment.Type
	switch comment.Type {
	case "review":
		typeDisplay = "PR Review"
	case "review_comment":
		typeDisplay = "Review Comment"
	case "issue_comment":
		typeDisplay = "General Comment"
	}
	prompt.WriteString(fmt.Sprintf("Type: %s\n", typeDisplay))

	if comment.Path != "" {
		prompt.WriteString(fmt.Sprintf("File: %s", comment.Path))
		if comment.Line > 0 {
			prompt.WriteString(fmt.Sprintf(" (line %d)", comment.Line))
		}
		prompt.WriteString("\n")
	}

	prompt.WriteString("\nComment:\n")
	prompt.WriteString("---\n")

	// Handle split comments differently
	if comment.IsSplit {
		// Only include accepted pieces
		acceptedPieces := comment.GetAcceptedPieces()
		if len(acceptedPieces) == 0 {
			// No accepted pieces, return empty prompt
			return ""
		}

		prompt.WriteString("Note: This comment has been split into pieces. Only the following selected pieces are included:\n\n")
		for i, piece := range acceptedPieces {
			if i > 0 {
				prompt.WriteString("\n\n")
			}
			prompt.WriteString(piece.Content)
		}
	} else {
		prompt.WriteString(comment.Body)
	}

	prompt.WriteString("\n---\n\n")

	// Customize instructions based on comment type
	switch comment.Type {
	case "review":
		prompt.WriteString("This is a general PR review. Please address the overall feedback provided. ")
	case "review_comment":
		prompt.WriteString("This is a line-specific review comment. Please address the specific code feedback. ")
	case "issue_comment":
		prompt.WriteString("This is a general PR discussion comment. Please respond appropriately. ")
	}

	prompt.WriteString("If the comment is asking a question, provide a clear answer. ")
	prompt.WriteString("If it's suggesting a change, implement it. ")
	prompt.WriteString("If you need clarification, explain what's unclear.")

	return prompt.String()
}
