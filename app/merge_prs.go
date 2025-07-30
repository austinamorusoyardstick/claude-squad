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

func (m *home) mergePRs(instance *session.Instance, selectedPRs []*git.PullRequest) tea.Cmd {
	// Show progress overlay
	progressText := fmt.Sprintf("Merging %d PRs...\n\n", len(selectedPRs))
	for i, pr := range selectedPRs {
		progressText += fmt.Sprintf("%d. PR #%d: %s\n", i+1, pr.Number, pr.Title)
	}
	m.textOverlay = overlay.NewTextOverlay(progressText)
	m.state = stateHelp

	// Return a command that performs the merge
	return m.createMergePRCmd(instance, selectedPRs)
}

func (m *home) performMergePRs(instance *session.Instance, selectedPRs []*git.PullRequest) error {
	worktree, err := instance.GetGitWorktree()
	if err != nil {
		return fmt.Errorf("failed to get git worktree: %w", err)
	}

	// Note: Not used but may be needed for future enhancements
	_ = worktree.GetWorktreePath()

	// Create a new branch name based on the PRs being merged
	timestamp := time.Now().Format("20060102-150405")
	prNumbers := make([]string, len(selectedPRs))
	for i, pr := range selectedPRs {
		prNumbers[i] = fmt.Sprintf("%d", pr.Number)
	}
	newBranchName := fmt.Sprintf("merge-prs-%s-%s", strings.Join(prNumbers, "-"), timestamp)

	// Get the repo path from the current worktree
	repoPath := worktree.GetRepoPath()
	
	// Create a new worktree for the merge
	mergeWorktree, _, err := git.NewGitWorktree(repoPath, newBranchName)
	if err != nil {
		return fmt.Errorf("failed to create merge worktree: %w", err)
	}

	// Set up the new worktree from main branch
	if err := mergeWorktree.Setup(); err != nil {
		return fmt.Errorf("failed to setup merge worktree: %w", err)
	}

	// Note: mergeWorktreePath not used yet but kept for potential future use
	_ = mergeWorktree.GetWorktreePath()

	// Cherry-pick or merge each PR's changes
	var successfulMerges []int
	var failedMerges []string
	
	for i, pr := range selectedPRs {
		// Send progress update
		progressMsg := fmt.Sprintf("Merging PR #%d (%d/%d): %s", pr.Number, i+1, len(selectedPRs), pr.Title)
		// Note: We can't update UI directly from goroutine, would need to send messages

		// Fetch the PR branch
		if _, err := mergeWorktree.FetchBranch(pr.HeadRef); err != nil {
			// Try to continue with other PRs
			failedMerges = append(failedMerges, fmt.Sprintf("PR #%d: Failed to fetch - %v", pr.Number, err))
			continue
		}

		// Cherry-pick the PR's commits
		if err := mergeWorktree.CherryPickBranch(pr.HeadRef); err != nil {
			// Check if it's a merge conflict
			if strings.Contains(err.Error(), "conflict") {
				failedMerges = append(failedMerges, fmt.Sprintf("PR #%d: Merge conflict", pr.Number))
			} else {
				failedMerges = append(failedMerges, fmt.Sprintf("PR #%d: %v", pr.Number, err))
			}
			continue
		}
		
		successfulMerges = append(successfulMerges, pr.Number)
	}

	// Only proceed if we have successful merges
	if len(successfulMerges) == 0 {
		return fmt.Errorf("no PRs were successfully merged")
	}

	// Create a commit message summarizing the merged PRs
	commitMessage := fmt.Sprintf("Merge %d PRs\n\n", len(successfulMerges))
	for _, pr := range selectedPRs {
		prNum := pr.Number
		// Check if this PR was successfully merged
		wasSuccessful := false
		for _, num := range successfulMerges {
			if num == prNum {
				wasSuccessful = true
				break
			}
		}
		if wasSuccessful {
			commitMessage += fmt.Sprintf("- PR #%d: %s\n", pr.Number, pr.Title)
		}
	}

	// Commit the merged changes
	if err := mergeWorktree.CommitMergedChanges(commitMessage); err != nil {
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

	// Return appropriate status
	if len(successfulMerges) == 0 {
		return fmt.Errorf("failed to merge any PRs")
	}
	
	// Note: In actual implementation, we'd send a message to update the UI
	// For now, we just return success
	fmt.Printf("Successfully merged %d PRs into branch %s\n", len(successfulMerges), newBranchName)
	if newPRNumber > 0 {
		fmt.Printf("Created PR #%d\n", newPRNumber)
	}
	if len(failedMerges) > 0 {
		fmt.Printf("Failed to merge %d PRs:\n", len(failedMerges))
		for _, failure := range failedMerges {
			fmt.Printf("  - %s\n", failure)
		}
	}

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