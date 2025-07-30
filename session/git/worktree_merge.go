package git

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// CherryPickBranch cherry-picks all commits from a branch
func (g *GitWorktree) CherryPickBranch(branchName string) error {
	// Get the merge base between current branch and the target branch
	mergeBase, err := g.runGitCommand(g.worktreePath, "merge-base", "HEAD", fmt.Sprintf("origin/%s", branchName))
	if err != nil {
		return fmt.Errorf("failed to find merge base: %w", err)
	}
	mergeBaseSHA := strings.TrimSpace(string(mergeBase))

	// Get all commits from the branch since merge base
	commits, err := g.runGitCommand(g.worktreePath, "rev-list", "--reverse", fmt.Sprintf("%s..origin/%s", mergeBaseSHA, branchName))
	if err != nil {
		return fmt.Errorf("failed to get commits: %w", err)
	}

	commitSHAs := strings.Split(strings.TrimSpace(string(commits)), "\n")
	if len(commitSHAs) == 0 || (len(commitSHAs) == 1 && commitSHAs[0] == "") {
		// No commits to cherry-pick
		return nil
	}

	// Cherry-pick each commit
	for _, sha := range commitSHAs {
		if sha == "" {
			continue
		}
		_, err := g.runGitCommand(g.worktreePath, "cherry-pick", sha)
		if err != nil {
			// Check if it's a conflict
			status, _ := g.runGitCommand(g.worktreePath, "status", "--porcelain")
			if strings.Contains(string(status), "UU") {
				// Abort the cherry-pick
				g.runGitCommand(g.worktreePath, "cherry-pick", "--abort")
				return fmt.Errorf("merge conflict while cherry-picking commit %s", sha)
			}
			return fmt.Errorf("failed to cherry-pick commit %s: %w", sha, err)
		}
	}

	return nil
}

// CommitMergedChanges creates a commit with the given message for merged changes
func (g *GitWorktree) CommitMergedChanges(message string) error {
	// Stage all changes
	if _, err := g.runGitCommand(g.worktreePath, "add", "-A"); err != nil {
		return fmt.Errorf("failed to stage changes: %w", err)
	}

	// Check if there are changes to commit
	status, err := g.runGitCommand(g.worktreePath, "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("failed to check status: %w", err)
	}

	if strings.TrimSpace(string(status)) == "" {
		// No changes to commit
		return nil
	}

	// Create commit
	if _, err := g.runGitCommand(g.worktreePath, "commit", "-m", message); err != nil {
		return fmt.Errorf("failed to create commit: %w", err)
	}

	return nil
}

// PushBranch pushes the current branch to origin
func (g *GitWorktree) PushBranch() error {
	// Push the branch
	if _, err := g.runGitCommand(g.worktreePath, "push", "-u", "origin", g.branchName); err != nil {
		return fmt.Errorf("failed to push branch: %w", err)
	}
	return nil
}

// CreatePullRequest creates a new pull request using gh CLI
func (g *GitWorktree) CreatePullRequest(title, body string) (int, error) {
	// Create the PR using gh CLI
	cmd := exec.Command("gh", "pr", "create",
		"--title", title,
		"--body", body,
		"--head", g.branchName,
	)
	cmd.Dir = g.worktreePath

	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to create pull request: %w", err)
	}

	// Parse the PR URL to get the number
	prURL := strings.TrimSpace(string(output))
	parts := strings.Split(prURL, "/")
	if len(parts) > 0 {
		prNumber := parts[len(parts)-1]
		// Convert to int
		var pr struct {
			Number int `json:"number"`
		}
		// Use gh to get the PR number
		cmd := exec.Command("gh", "pr", "view", prNumber, "--json", "number")
		cmd.Dir = g.worktreePath
		output, err := cmd.Output()
		if err == nil {
			if err := json.Unmarshal(output, &pr); err == nil {
				return pr.Number, nil
			}
		}
	}

	// Fallback: just return 0 if we can't parse the number
	return 0, nil
}
