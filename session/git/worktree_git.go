package git

import (
	"claude-squad/config"
	"claude-squad/log"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// runGitCommand executes a git command and returns any error
func (g *GitWorktree) runGitCommand(path string, args ...string) (string, error) {
	// Check if the path exists before running git command
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", fmt.Errorf("directory does not exist: %s", path)
	}

	baseArgs := []string{"-C", path}
	cmd := exec.Command("git", append(baseArgs, args...)...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Include the full command in the error for debugging
		fullCmd := fmt.Sprintf("git %s", strings.Join(append(baseArgs, args...), " "))
		// Trim the output to avoid extremely long error messages, but keep enough for debugging
		outputStr := string(output)
		if len(outputStr) > 500 {
			outputStr = outputStr[:500] + "... (truncated)"
		}
		return "", fmt.Errorf("git command failed: %s\nCommand: %s\nError: %w", outputStr, fullCmd, err)
	}

	return string(output), nil
}

// PushChanges commits and pushes changes in the worktree to the remote branch
func (g *GitWorktree) PushChanges(commitMessage string, open bool) error {
	if err := checkGHCLI(); err != nil {
		return err
	}

	// Check if there are any changes to commit
	isDirty, err := g.IsDirty()
	if err != nil {
		return fmt.Errorf("failed to check for changes: %w", err)
	}

	if isDirty {
		// Stage all changes
		if _, err := g.runGitCommand(g.worktreePath, "add", "."); err != nil {
			log.ErrorLog.Print(err)
			return fmt.Errorf("failed to stage changes: %w", err)
		}

		// Create commit
		if _, err := g.runGitCommand(g.worktreePath, "commit", "-m", commitMessage, "--no-verify"); err != nil {
			log.ErrorLog.Print(err)
			return fmt.Errorf("failed to commit changes: %w", err)
		}
	}

	// First push the branch to remote to ensure it exists
	pushCmd := exec.Command("gh", "repo", "sync", "--source", "-b", g.branchName)
	pushCmd.Dir = g.worktreePath
	if err := pushCmd.Run(); err != nil {
		// If sync fails, try creating the branch on remote first
		gitPushCmd := exec.Command("git", "push", "-u", "origin", g.branchName)
		gitPushCmd.Dir = g.worktreePath
		if pushOutput, pushErr := gitPushCmd.CombinedOutput(); pushErr != nil {
			log.ErrorLog.Print(pushErr)
			return fmt.Errorf("failed to push branch: %s (%w)", pushOutput, pushErr)
		}
	}

	// Now sync with remote
	syncCmd := exec.Command("gh", "repo", "sync", "-b", g.branchName)
	syncCmd.Dir = g.worktreePath
	if output, err := syncCmd.CombinedOutput(); err != nil {
		log.ErrorLog.Print(err)
		return fmt.Errorf("failed to sync changes: %s (%w)", output, err)
	}

	// Open the branch in the browser
	if open {
		if err := g.OpenBranchURL(); err != nil {
			// Just log the error but don't fail the push operation
			log.ErrorLog.Printf("failed to open branch URL: %v", err)
		}
	}

	return nil
}

// CommitChanges commits changes locally without pushing to remote
func (g *GitWorktree) CommitChanges(commitMessage string) error {
	// Check if there are any changes to commit
	isDirty, err := g.IsDirty()
	if err != nil {
		return fmt.Errorf("failed to check for changes: %w", err)
	}

	if isDirty {
		// Stage all changes
		if _, err := g.runGitCommand(g.worktreePath, "add", "."); err != nil {
			log.ErrorLog.Print(err)
			return fmt.Errorf("failed to stage changes: %w", err)
		}

		// Create commit (local only)
		if _, err := g.runGitCommand(g.worktreePath, "commit", "-m", commitMessage, "--no-verify"); err != nil {
			log.ErrorLog.Print(err)
			return fmt.Errorf("failed to commit changes: %w", err)
		}
	}

	return nil
}

// IsDirty checks if the worktree has uncommitted changes
func (g *GitWorktree) IsDirty() (bool, error) {
	output, err := g.runGitCommand(g.worktreePath, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("failed to check worktree status: %w", err)
	}
	return len(output) > 0, nil
}

// IsBranchCheckedOut checks if the instance branch is currently checked out
func (g *GitWorktree) IsBranchCheckedOut() (bool, error) {
	// If worktree doesn't exist, the branch can't be checked out there
	if _, err := os.Stat(g.worktreePath); os.IsNotExist(err) {
		// Check in the main repo instead
		output, err := g.runGitCommand(g.repoPath, "branch", "--show-current")
		if err != nil {
			// If we can't check, assume it's not checked out to be safe
			if strings.Contains(err.Error(), "directory does not exist") {
				return false, nil
			}
			return false, fmt.Errorf("failed to get current branch: %w", err)
		}
		return strings.TrimSpace(string(output)) == g.branchName, nil
	}

	// Check if branch is checked out in the worktree
	output, err := g.runGitCommand(g.worktreePath, "branch", "--show-current")
	if err != nil {
		// If worktree path doesn't exist anymore, it's not checked out
		if strings.Contains(err.Error(), "directory does not exist") {
			return false, nil
		}
		return false, fmt.Errorf("failed to get current branch in worktree: %w", err)
	}
	return strings.TrimSpace(string(output)) == g.branchName, nil
}

// OpenBranchURL opens the branch URL in the default browser
func (g *GitWorktree) OpenBranchURL() error {
	// Check if GitHub CLI is available
	if err := checkGHCLI(); err != nil {
		return err
	}

	cmd := exec.Command("gh", "browse", "--branch", g.branchName)
	cmd.Dir = g.worktreePath
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to open branch URL: %w", err)
	}
	return nil
}

// RebaseWithMain rebases the current branch with the main branch
func (g *GitWorktree) RebaseWithMain() error {
	// First, create a backup branch with a unique name
	timestamp := time.Now().Unix()
	backupBranch := fmt.Sprintf("%s-backup-%d", g.branchName, timestamp)

	// Ensure the backup branch name is unique by checking if it exists
	for {
		// Check if the branch already exists locally or remotely
		localExists := false
		remoteExists := false

		if _, err := g.runGitCommand(g.worktreePath, "rev-parse", "--verify", backupBranch); err == nil {
			localExists = true
		}
		if _, err := g.runGitCommand(g.worktreePath, "rev-parse", "--verify", fmt.Sprintf("origin/%s", backupBranch)); err == nil {
			remoteExists = true
		}

		if !localExists && !remoteExists {
			break
		}

		// If it exists, add a counter to make it unique
		timestamp++
		backupBranch = fmt.Sprintf("%s-backup-%d", g.branchName, timestamp)
	}

	if _, err := g.runGitCommand(g.worktreePath, "branch", backupBranch); err != nil {
		return fmt.Errorf("failed to create backup branch: %w", err)
	}

	// Push the backup branch with --no-verify for speed
	if _, err := g.runGitCommand(g.worktreePath, "push", "origin", backupBranch, "--no-verify"); err != nil {
		// If push fails, just log it but continue
		log.WarningLog.Printf("failed to push backup branch %s: %v", backupBranch, err)
	}

	// Fetch the latest from origin
	if _, err := g.runGitCommand(g.worktreePath, "fetch", "origin"); err != nil {
		return fmt.Errorf("failed to fetch from origin: %w", err)
	}

	// Determine the main branch name using git remote show origin
	mainBranch := "main"
	cmd := exec.Command("sh", "-c", "git remote show origin | sed -n '/HEAD branch/s/.*: //p'")
	cmd.Dir = g.worktreePath
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		mainBranch = strings.TrimSpace(string(output))
	} else {
		// Fallback: Try common defaults if the command fails
		if _, err := g.runGitCommand(g.worktreePath, "rev-parse", "origin/main"); err != nil {
			if _, err := g.runGitCommand(g.worktreePath, "rev-parse", "origin/master"); err == nil {
				mainBranch = "master"
			} else if _, err := g.runGitCommand(g.worktreePath, "rev-parse", "origin/dev"); err == nil {
				mainBranch = "dev"
			}
		}
	}

	// Perform the rebase
	if _, err := g.runGitCommand(g.worktreePath, "rebase", fmt.Sprintf("origin/%s", mainBranch)); err != nil {
		// Check if this is a merge conflict by examining the git status
		if g.hasMergeConflicts() {
			// Open IDE with the conflicted files - for now, load config here since we can't change the function signature easily
			globalConfig := config.LoadConfig()
			if ideErr := g.openIdeForConflicts(globalConfig); ideErr != nil {
				// If IDE fails to open, still return the conflict info
				log.WarningLog.Printf("Failed to open IDE for conflict resolution: %v", ideErr)
			}
			return fmt.Errorf("merge conflicts detected during rebase with origin/%s. IDE opened for conflict resolution. Backup branch created: %s", mainBranch, backupBranch)
		}

		// If it's not a merge conflict, abort the rebase as before
		g.runGitCommand(g.worktreePath, "rebase", "--abort")
		return fmt.Errorf("rebase failed with origin/%s. Backup branch created: %s", mainBranch, backupBranch)
	}

	return nil
}

// hasMergeConflicts checks if there are currently merge conflicts in the worktree
func (g *GitWorktree) hasMergeConflicts() bool {
	// Check git status for conflict markers
	output, err := g.runGitCommand(g.worktreePath, "status", "--porcelain")
	if err != nil {
		return false
	}

	// Look for files with conflict status (UU, AA, etc.)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if len(line) >= 2 {
			status := line[:2]
			// Common conflict statuses: UU (both modified), AA (both added), etc.
			if strings.Contains(status, "U") || status == "AA" || status == "DD" {
				return true
			}
		}
	}

	return false
}

// openIdeForConflicts opens the configured IDE at the worktree path for conflict resolution
func (g *GitWorktree) openIdeForConflicts(globalConfig *config.Config) error {
	// Get the IDE command from configuration
	ideCommand := config.GetEffectiveIdeCommand(g.repoPath, globalConfig)
	
	// Open IDE at the worktree path
	cmd := exec.Command(ideCommand, g.worktreePath)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to open IDE (%s): %w", ideCommand, err)
	}

	log.InfoLog.Printf("IDE (%s) opened for conflict resolution at: %s", ideCommand, g.worktreePath)
	return nil
}

// IsRebaseInProgress checks if a rebase is currently in progress
func (g *GitWorktree) IsRebaseInProgress() bool {
	// Check if .git/rebase-merge or .git/rebase-apply directories exist
	rebaseMergePath := fmt.Sprintf("%s/.git/rebase-merge", g.worktreePath)
	rebaseApplyPath := fmt.Sprintf("%s/.git/rebase-apply", g.worktreePath)

	if _, err := os.Stat(rebaseMergePath); err == nil {
		return true
	}
	if _, err := os.Stat(rebaseApplyPath); err == nil {
		return true
	}

	return false
}

// ContinueRebase continues a rebase after conflicts have been resolved
func (g *GitWorktree) ContinueRebase() error {
	// Check if rebase is in progress
	if !g.IsRebaseInProgress() {
		return fmt.Errorf("no rebase in progress")
	}

	// Check if there are still conflicts
	if g.hasMergeConflicts() {
		return fmt.Errorf("merge conflicts still exist, please resolve them first")
	}

	// Stage all resolved files
	if _, err := g.runGitCommand(g.worktreePath, "add", "."); err != nil {
		return fmt.Errorf("failed to stage resolved files: %w", err)
	}

	// Continue the rebase
	if _, err := g.runGitCommand(g.worktreePath, "rebase", "--continue"); err != nil {
		return fmt.Errorf("failed to continue rebase: %w", err)
	}

	return nil
}

// AbortRebase aborts the current rebase and returns to the original state
func (g *GitWorktree) AbortRebase() error {
	if !g.IsRebaseInProgress() {
		return fmt.Errorf("no rebase in progress")
	}

	if _, err := g.runGitCommand(g.worktreePath, "rebase", "--abort"); err != nil {
		return fmt.Errorf("failed to abort rebase: %w", err)
	}

	return nil
}

// GetCurrentBranch returns the current branch name
func (g *GitWorktree) GetCurrentBranch() (string, error) {
	return g.branchName, nil
}

// FindLastBookmarkCommit finds the last commit with [BOOKMARK] prefix on the given branch
func (g *GitWorktree) FindLastBookmarkCommit(branchName string) (string, error) {
	// Search for the last bookmark commit on the branch
	output, err := g.runGitCommand(g.worktreePath, "log", "--oneline", "--grep=^\\[BOOKMARK\\]", "-n", "1", "--format=%H", branchName)
	if err != nil {
		// If no bookmark found, return empty string (not an error).
		// `git log` returns a non-zero exit code when no commits match.
		if strings.Contains(err.Error(), "does not have any commits") || output == "" {
			return "", nil
		}
		// For other errors, return the error.
		return "", fmt.Errorf("failed to find last bookmark commit: %w", err)
	}

	return strings.TrimSpace(output), nil
}

// GetCommitMessagesSince gets all commit messages since a given SHA on the branch
func (g *GitWorktree) GetCommitMessagesSince(sinceSHA string, branchName string) ([]string, error) {
	var args []string
	if sinceSHA == "" {
		// If no previous bookmark, get all commits on this branch
		args = []string{"log", "--oneline", "--format=%s", branchName}
	} else {
		// Get commits since the last bookmark
		args = []string{"log", "--oneline", "--format=%s", fmt.Sprintf("%s..%s", sinceSHA, branchName)}
	}

	output, err := g.runGitCommand(g.worktreePath, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit messages: %w", err)
	}

	if output == "" {
		return []string{}, nil
	}

	// Split by newline and filter out empty lines and bookmark commits
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var messages []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "[BOOKMARK]") {
			messages = append(messages, line)
		}
	}

	return messages, nil
}

// CreateBookmarkCommit creates an empty commit with the bookmark message
func (g *GitWorktree) CreateBookmarkCommit(message string) error {
	// Create an empty commit with the bookmark message
	_, err := g.runGitCommand(g.worktreePath, "commit", "--allow-empty", "-m", message)
	if err != nil {
		return fmt.Errorf("failed to create bookmark commit: %w", err)
	}

	return nil
}
