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

type mergePRsStartedMsg struct{}

type mergePRsProgressMsg struct {
	current int
	total   int
	status  string
	pr      *git.PullRequest
}

type mergePRsCompletedMsg struct {
	newBranch string
	prs       []*git.PullRequest
	err       error
}

func (m *home) mergePRs(instance *session.Instance, selectedPRs []*git.PullRequest) {
	// Show progress overlay
	progressText := fmt.Sprintf("Merging %d PRs...\n\n", len(selectedPRs))
	for i, pr := range selectedPRs {
		progressText += fmt.Sprintf("%d. PR #%d: %s\n", i+1, pr.Number, pr.Title)
	}
	m.textOverlay = overlay.NewTextOverlay(progressText)
	m.state = stateHelp

	// Start the merge process asynchronously
	go func() {
		if err := m.performMergePRs(instance, selectedPRs); err != nil {
			m.handleError(err)
		}
	}()
}

func (m *home) performMergePRs(instance *session.Instance, selectedPRs []*git.PullRequest) error {
	worktree, err := instance.GetGitWorktree()
	if err != nil {
		return fmt.Errorf("failed to get git worktree: %w", err)
	}

	worktreePath := worktree.GetWorktreePath()

	// Create a new branch name based on the PRs being merged
	timestamp := time.Now().Format("20060102-150405")
	prNumbers := make([]string, len(selectedPRs))
	for i, pr := range selectedPRs {
		prNumbers[i] = fmt.Sprintf("%d", pr.Number)
	}
	newBranchName := fmt.Sprintf("merge-prs-%s-%s", strings.Join(prNumbers, "-"), timestamp)

	// Create a new worktree for the merge
	mergeWorktree, err := git.NewGitWorktree(newBranchName)
	if err != nil {
		return fmt.Errorf("failed to create merge worktree: %w", err)
	}

	// Set up the new worktree from main branch
	if err := mergeWorktree.Setup(); err != nil {
		return fmt.Errorf("failed to setup merge worktree: %w", err)
	}

	mergeWorktreePath := mergeWorktree.GetWorktreePath()

	// Cherry-pick or merge each PR's changes
	for i, pr := range selectedPRs {
		// Send progress update
		progressMsg := fmt.Sprintf("Merging PR #%d (%d/%d): %s", pr.Number, i+1, len(selectedPRs), pr.Title)
		m.textOverlay.SetContent(progressMsg)

		// Fetch the PR branch
		if err := mergeWorktree.FetchBranch(pr.HeadRef); err != nil {
			// Try to continue with other PRs
			progressMsg += fmt.Sprintf("\nWarning: Failed to fetch PR #%d: %v", pr.Number, err)
			m.textOverlay.SetContent(progressMsg)
			continue
		}

		// Cherry-pick the PR's commits
		if err := mergeWorktree.CherryPickBranch(pr.HeadRef); err != nil {
			// Check if it's a merge conflict
			if strings.Contains(err.Error(), "conflict") {
				progressMsg += fmt.Sprintf("\nConflict in PR #%d - manual resolution required", pr.Number)
				m.textOverlay.SetContent(progressMsg)
				// Continue with other PRs
				continue
			}
			return fmt.Errorf("failed to cherry-pick PR #%d: %w", pr.Number, err)
		}
	}

	// Create a commit message summarizing the merged PRs
	commitMessage := fmt.Sprintf("Merge %d PRs\n\n", len(selectedPRs))
	for _, pr := range selectedPRs {
		commitMessage += fmt.Sprintf("- PR #%d: %s\n", pr.Number, pr.Title)
	}

	// Commit the merged changes
	if err := mergeWorktree.CommitChanges(commitMessage); err != nil {
		return fmt.Errorf("failed to commit merged changes: %w", err)
	}

	// Push the new branch
	if err := mergeWorktree.PushBranch(); err != nil {
		return fmt.Errorf("failed to push merged branch: %w", err)
	}

	// Create a new PR from the merged branch
	prTitle := fmt.Sprintf("Merge PRs: %s", strings.Join(prNumbers, ", "))
	prBody := fmt.Sprintf("This PR merges the following PRs:\n\n")
	for _, pr := range selectedPRs {
		prBody += fmt.Sprintf("- #%d: %s\n", pr.Number, pr.Title)
	}
	prBody += fmt.Sprintf("\n\nCreated by claude-squad merge-prs feature")

	newPRNumber, err := mergeWorktree.CreatePullRequest(prTitle, prBody)
	if err != nil {
		return fmt.Errorf("failed to create pull request: %w", err)
	}

	// Success! Update the overlay
	successMsg := fmt.Sprintf("Successfully merged %d PRs!\n\n", len(selectedPRs))
	successMsg += fmt.Sprintf("New branch: %s\n", newBranchName)
	successMsg += fmt.Sprintf("New PR: #%d\n\n", newPRNumber)
	successMsg += "Press any key to continue..."
	m.textOverlay.SetContent(successMsg)

	// Clean up the merge worktree
	if err := mergeWorktree.Cleanup(); err != nil {
		// Log but don't fail
		fmt.Printf("Warning: failed to cleanup merge worktree: %v\n", err)
	}

	return nil
}

// Update the git worktree to add missing methods
func (m *home) createMergePRCmd(instance *session.Instance, selectedPRs []*git.PullRequest) tea.Cmd {
	return func() tea.Msg {
		// Perform the merge operation
		err := m.performMergePRs(instance, selectedPRs)
		
		// Return completion message
		return mergePRsCompletedMsg{
			prs: selectedPRs,
			err: err,
		}
	}
}