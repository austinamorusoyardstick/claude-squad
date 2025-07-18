package git

import (
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
		// If rebase fails, try to abort and restore
		g.runGitCommand(g.worktreePath, "rebase", "--abort")
		return fmt.Errorf("rebase failed with origin/%s. Backup branch created: %s", mainBranch, backupBranch)
	}

	return nil
}
