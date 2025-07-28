package git

import (
	"claude-squad/config"
	"claude-squad/log"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// RebaseConflictError is returned when rebase conflicts occur and polling is needed
type RebaseConflictError struct {
	TempDir    string
	MainBranch string
	Message    string
	Worktree   *GitWorktree
}

func (e *RebaseConflictError) Error() string {
	return e.Message
}

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
		// Abort the rebase in worktree
		g.runGitCommand(g.worktreePath, "rebase", "--abort")
		
		// Always use clone approach for any rebase failure (including conflicts)
		log.InfoLog.Printf("Rebase failed in worktree, using clone approach")
		if cloneErr := g.rebaseWithClone(mainBranch, backupBranch); cloneErr != nil {
			return fmt.Errorf("rebase failed with origin/%s. Backup branch created: %s. Error: %w", mainBranch, backupBranch, cloneErr)
		}
		
		return nil
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

// rebaseWithClone attempts to perform a rebase in a fresh clone of the repository
func (g *GitWorktree) rebaseWithClone(mainBranch, backupBranch string) error {
	// Sanitize branch name for use in temp directory name (replace path separators)
	sanitizedBranch := strings.ReplaceAll(g.branchName, "/", "-")
	
	// Create a temporary directory for the clone
	tempDir, err := os.MkdirTemp("", fmt.Sprintf("claude-squad-rebase-%s-*", sanitizedBranch))
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	
	log.InfoLog.Printf("Created temporary clone directory: %s", tempDir)
	
	// Get the remote URL
	remoteURL, err := g.runGitCommand(g.worktreePath, "remote", "get-url", "origin")
	if err != nil {
		os.RemoveAll(tempDir)
		return fmt.Errorf("failed to get remote URL: %w", err)
	}
	remoteURL = strings.TrimSpace(remoteURL)
	
	// Clone the repository
	log.InfoLog.Printf("Cloning repository to temp directory...")
	cloneCmd := exec.Command("git", "clone", remoteURL, tempDir)
	if output, err := cloneCmd.CombinedOutput(); err != nil {
		os.RemoveAll(tempDir)
		return fmt.Errorf("failed to clone repository: %s (%w)", output, err)
	}
	
	// Checkout the branch in the clone
	if _, err := g.runGitCommand(tempDir, "checkout", g.branchName); err != nil {
		os.RemoveAll(tempDir)
		return fmt.Errorf("failed to checkout branch %s in clone: %w", g.branchName, err)
	}
	
	// Attempt rebase in the clone
	if _, err := g.runGitCommand(tempDir, "rebase", fmt.Sprintf("origin/%s", mainBranch)); err != nil {
		// Check if this is a merge conflict
		if g.hasMergeConflictsInPath(tempDir) {
			// Open IDE with the conflicted files in temp directory
			globalConfig := config.LoadConfig()
			ideCommand := config.GetEffectiveIdeCommand(g.repoPath, globalConfig)
			
			cmd := exec.Command(ideCommand, tempDir)
			if ideErr := cmd.Start(); ideErr != nil {
				log.WarningLog.Printf("Failed to open IDE for conflict resolution in temp clone: %v", ideErr)
			} else {
				log.InfoLog.Printf("IDE (%s) opened for conflict resolution at temp clone: %s", ideCommand, tempDir)
			}
			
			// Don't remove temp dir - user needs to resolve conflicts
			// Create a marker file to help with completion detection
			markerPath := fmt.Sprintf("%s/.claude-squad-rebase-in-progress", tempDir)
			if err := os.WriteFile(markerPath, []byte("Remove this file after completing rebase to trigger sync\n"), 0644); err != nil {
				log.WarningLog.Printf("Failed to create rebase marker file: %v", err)
			}
			
			return &RebaseConflictError{
				TempDir:    tempDir,
				MainBranch: mainBranch,
				Message:    fmt.Sprintf("merge conflicts detected during rebase. IDE opened at %s. Monitoring for completion...", tempDir),
				Worktree:   g,
			}
		}
		
		// If it's not a merge conflict, abort and clean up
		g.runGitCommand(tempDir, "rebase", "--abort")
		os.RemoveAll(tempDir)
		return fmt.Errorf("rebase failed in clone as well")
	}
	
	// Rebase succeeded in clone - now we need to copy the changes back
	log.InfoLog.Printf("Rebase succeeded in clone, copying changes back to worktree...")
	
	// Get the new commit SHA after rebase
	newSHA, err := g.runGitCommand(tempDir, "rev-parse", "HEAD")
	if err != nil {
		os.RemoveAll(tempDir)
		return fmt.Errorf("failed to get new commit SHA: %w", err)
	}
	newSHA = strings.TrimSpace(newSHA)
	
	// Force update the branch in the worktree to match the rebased state
	if _, err := g.runGitCommand(g.worktreePath, "fetch", "origin"); err != nil {
		os.RemoveAll(tempDir)
		return fmt.Errorf("failed to fetch after clone rebase: %w", err)
	}
	
	// First push the rebased branch from the clone
	if _, err := g.runGitCommand(tempDir, "push", "--force-with-lease", "origin", g.branchName); err != nil {
		os.RemoveAll(tempDir)
		return fmt.Errorf("failed to push rebased branch from clone: %w", err)
	}
	
	// Now reset the worktree to the rebased state
	if _, err := g.runGitCommand(g.worktreePath, "fetch", "origin", g.branchName); err != nil {
		os.RemoveAll(tempDir)
		return fmt.Errorf("failed to fetch rebased branch: %w", err)
	}
	
	if _, err := g.runGitCommand(g.worktreePath, "reset", "--hard", fmt.Sprintf("origin/%s", g.branchName)); err != nil {
		os.RemoveAll(tempDir)
		return fmt.Errorf("failed to reset worktree to rebased state: %w", err)
	}
	
	// Clean up temp directory
	os.RemoveAll(tempDir)
	log.InfoLog.Printf("Successfully completed rebase using clone approach")
	
	return nil
}

// hasMergeConflictsInPath checks if there are merge conflicts in a specific path
func (g *GitWorktree) hasMergeConflictsInPath(path string) bool {
	// Check git status for conflict markers
	output, err := g.runGitCommand(path, "status", "--porcelain")
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

// CreateRebasePollingCommand creates a tea.Cmd for polling rebase completion
func (g *GitWorktree) CreateRebasePollingCommand(tempDir string, mainBranch string) func() interface{} {
	log.InfoLog.Printf("CreateRebasePollingCommand called for %s", tempDir)
	return func() interface{} {
		log.InfoLog.Printf("Polling function executing - checking rebase completion status in %s", tempDir)
		
		// Check if directory still exists
		if _, err := os.Stat(tempDir); os.IsNotExist(err) {
			log.InfoLog.Printf("Temp directory %s no longer exists", tempDir)
			return RebasePollingMsg{
				Status:     "cancelled",
				TempDir:    tempDir,
				MainBranch: mainBranch,
				Worktree:   g,
			}
		}
		
		// Check for our marker file - if it's been removed, consider rebase complete
		markerPath := fmt.Sprintf("%s/.claude-squad-rebase-in-progress", tempDir)
		if _, err := os.Stat(markerPath); os.IsNotExist(err) {
			log.InfoLog.Printf("Marker file removed, considering rebase complete")
			// Skip other checks and proceed to sync
		} else {
			// Marker still exists, do normal checks
			
			// First, let's check git status to see what git thinks
			gitStatus, statusErr := g.runGitCommand(tempDir, "status")
			if statusErr != nil {
				log.ErrorLog.Printf("Failed to get git status: %v", statusErr)
			} else {
				// Check if the status contains rebase-related messages
				statusStr := strings.ToLower(gitStatus)
				if strings.Contains(statusStr, "rebase in progress") || 
				   strings.Contains(statusStr, "you are currently rebasing") ||
				   strings.Contains(statusStr, "rebase --continue") {
					log.InfoLog.Printf("Git status indicates rebase still in progress")
					return RebasePollingMsg{
						Status:     "in_progress",
						TempDir:    tempDir,
						MainBranch: mainBranch,
						Worktree:   g,
					}
				}
			}
		}
		
		// Check if rebase is still in progress by looking for directories
		rebaseInProgress := g.isRebaseInProgressAtPath(tempDir)
		log.InfoLog.Printf("Checking rebase status via directories: in progress = %v", rebaseInProgress)
		
		if rebaseInProgress {
			log.InfoLog.Printf("Rebase still in progress at %s", tempDir)
			return RebasePollingMsg{
				Status:     "in_progress",
				TempDir:    tempDir,
				MainBranch: mainBranch,
				Worktree:   g,
			}
		}
		
		// Check if there are still conflicts
		hasConflicts := g.hasMergeConflictsInPath(tempDir)
		log.InfoLog.Printf("Checking for conflicts: has conflicts = %v", hasConflicts)
		
		if hasConflicts {
			log.InfoLog.Printf("Conflicts still exist at %s", tempDir)
			return RebasePollingMsg{
				Status:     "in_progress",
				TempDir:    tempDir,
				MainBranch: mainBranch,
				Worktree:   g,
			}
		}
		
		// Double-check the branch status in the clone
		status, err := g.runGitCommand(tempDir, "status", "--porcelain")
		if err != nil {
			log.ErrorLog.Printf("Failed to get status in clone: %v", err)
		} else {
			log.InfoLog.Printf("Clone git status output: '%s' (length: %d)", status, len(strings.TrimSpace(status)))
		}
		
		// Also check if we're still on the right branch
		currentBranch, err := g.runGitCommand(tempDir, "rev-parse", "--abbrev-ref", "HEAD")
		if err != nil {
			log.ErrorLog.Printf("Failed to get current branch: %v", err) 
		} else {
			currentBranch = strings.TrimSpace(currentBranch)
			log.InfoLog.Printf("Clone is on branch: %s", currentBranch)
		}
		
		// Check the rebase status more thoroughly
		rebaseOutput, _ := g.runGitCommand(tempDir, "status", "--short")
		log.InfoLog.Printf("Short status output: '%s'", strings.TrimSpace(rebaseOutput))
		
		// Check if any git process is still running in the directory
		// This helps avoid race conditions where git hasn't cleaned up yet
		gitProcessCheck, _ := exec.Command("sh", "-c", fmt.Sprintf("lsof +D %s 2>/dev/null | grep -E '(git|git-rebase)' || true", tempDir)).Output()
		if len(strings.TrimSpace(string(gitProcessCheck))) > 0 {
			log.InfoLog.Printf("Git process still running in directory, continuing to poll")
			return RebasePollingMsg{
				Status:     "in_progress",
				TempDir:    tempDir,
				MainBranch: mainBranch,
				Worktree:   g,
			}
		}
		
		// Final check - make sure we can actually read the HEAD
		headSHA, err := g.runGitCommand(tempDir, "rev-parse", "HEAD")
		if err != nil {
			log.ErrorLog.Printf("Failed to read HEAD after rebase: %v", err)
			// This might mean git is in a weird state, continue polling
			return RebasePollingMsg{
				Status:     "in_progress",
				TempDir:    tempDir,
				MainBranch: mainBranch,
				Worktree:   g,
			}
		}
		log.InfoLog.Printf("HEAD SHA after rebase: %s", strings.TrimSpace(headSHA))
		
		// Rebase appears to be complete - sync back to worktree
		log.InfoLog.Printf("All checks passed - rebase completed in clone, syncing back to worktree")
		
		if err := g.syncRebaseFromClone(tempDir, mainBranch); err != nil {
			log.ErrorLog.Printf("Failed to sync rebase from clone: %v", err)
			return RebasePollingMsg{
				Status:     "sync_failed",
				Error:      err,
				TempDir:    tempDir,
				MainBranch: mainBranch,
				Worktree:   g,
			}
		}
		
		// Success - clean up and exit
		os.RemoveAll(tempDir)
		log.InfoLog.Printf("Successfully synced rebase from clone and cleaned up temp directory")
		return RebasePollingMsg{
			Status:     "completed",
			TempDir:    tempDir,
			MainBranch: mainBranch,
			Worktree:   g,
		}
	}
}

// RebasePollingMsg represents the status of rebase polling
type RebasePollingMsg struct {
	Status     string // "in_progress", "completed", "sync_failed", "cancelled"
	Error      error
	TempDir    string
	MainBranch string
	Worktree   *GitWorktree
}

// isRebaseInProgressAtPath checks if a rebase is in progress at a specific path
func (g *GitWorktree) isRebaseInProgressAtPath(path string) bool {
	// Check if .git/rebase-merge or .git/rebase-apply directories exist
	rebaseMergePath := fmt.Sprintf("%s/.git/rebase-merge", path)
	rebaseApplyPath := fmt.Sprintf("%s/.git/rebase-apply", path)
	
	log.InfoLog.Printf("Checking for rebase directories: %s and %s", rebaseMergePath, rebaseApplyPath)
	
	if _, err := os.Stat(rebaseMergePath); err == nil {
		log.InfoLog.Printf("Found rebase-merge directory at %s", rebaseMergePath)
		return true
	}
	if _, err := os.Stat(rebaseApplyPath); err == nil {
		log.InfoLog.Printf("Found rebase-apply directory at %s", rebaseApplyPath)
		return true
	}
	
	log.InfoLog.Printf("No rebase directories found, rebase appears complete")
	return false
}

// syncRebaseFromClone syncs the completed rebase from clone back to worktree
func (g *GitWorktree) syncRebaseFromClone(tempDir string, mainBranch string) error {
	log.InfoLog.Printf("Starting local sync from clone %s to worktree %s", tempDir, g.worktreePath)
	
	// Get the current branch name in the clone to ensure we're on the right branch
	currentBranch, err := g.runGitCommand(tempDir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return fmt.Errorf("failed to get current branch in clone: %w", err)
	}
	currentBranch = strings.TrimSpace(currentBranch)
	log.InfoLog.Printf("Clone is on branch: %s (expected: %s)", currentBranch, g.branchName)
	
	// Get the new commit SHA after rebase
	newSHA, err := g.runGitCommand(tempDir, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("failed to get new commit SHA: %w", err)
	}
	newSHA = strings.TrimSpace(newSHA)
	log.InfoLog.Printf("New SHA after rebase in clone: %s", newSHA)
	
	// Get the old SHA in worktree for comparison
	oldSHA, _ := g.runGitCommand(g.worktreePath, "rev-parse", "HEAD")
	oldSHA = strings.TrimSpace(oldSHA)
	log.InfoLog.Printf("Current worktree SHA: %s", oldSHA)
	
	// Add the clone as a temporary remote in the worktree
	tempRemoteName := fmt.Sprintf("temp-clone-%d", time.Now().Unix())
	log.InfoLog.Printf("Adding temporary remote '%s' pointing to clone", tempRemoteName)
	
	// Add the clone directory as a remote
	if _, err := g.runGitCommand(g.worktreePath, "remote", "add", tempRemoteName, tempDir); err != nil {
		return fmt.Errorf("failed to add clone as remote: %w", err)
	}
	
	// Ensure we remove the temporary remote when done
	defer func() {
		log.InfoLog.Printf("Removing temporary remote '%s'", tempRemoteName)
		if _, err := g.runGitCommand(g.worktreePath, "remote", "remove", tempRemoteName); err != nil {
			log.WarningLog.Printf("Failed to remove temporary remote: %v", err)
		}
	}()
	
	// Fetch from the clone
	log.InfoLog.Printf("Fetching from clone via temporary remote")
	fetchOutput, err := g.runGitCommand(g.worktreePath, "fetch", tempRemoteName, g.branchName)
	if err != nil {
		return fmt.Errorf("failed to fetch from clone: %w", err)
	}
	log.InfoLog.Printf("Fetch output: %s", strings.TrimSpace(fetchOutput))
	
	// Check current branch in worktree
	worktreeBranch, err := g.runGitCommand(g.worktreePath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		log.WarningLog.Printf("Failed to get current branch in worktree: %v", err)
	} else {
		worktreeBranch = strings.TrimSpace(worktreeBranch)
		log.InfoLog.Printf("Worktree is on branch: %s", worktreeBranch)
		
		// If we're on a detached HEAD or wrong branch, checkout the right branch first
		if worktreeBranch == "HEAD" || worktreeBranch != g.branchName {
			log.InfoLog.Printf("Checking out branch %s in worktree first", g.branchName)
			checkoutOutput, err := g.runGitCommand(g.worktreePath, "checkout", g.branchName)
			if err != nil {
				log.WarningLog.Printf("Failed to checkout branch, will try to continue: %v", err)
			} else {
				log.InfoLog.Printf("Checkout output: %s", strings.TrimSpace(checkoutOutput))
			}
		}
	}
	
	// Reset the worktree to the rebased state from the clone
	resetRef := fmt.Sprintf("%s/%s", tempRemoteName, g.branchName)
	log.InfoLog.Printf("Resetting worktree to %s", resetRef)
	resetOutput, err := g.runGitCommand(g.worktreePath, "reset", "--hard", resetRef)
	if err != nil {
		return fmt.Errorf("failed to reset worktree to rebased state: %w", err)
	}
	log.InfoLog.Printf("Reset output: %s", strings.TrimSpace(resetOutput))
	
	// Verify the reset worked
	finalSHA, _ := g.runGitCommand(g.worktreePath, "rev-parse", "HEAD")
	finalSHA = strings.TrimSpace(finalSHA)
	log.InfoLog.Printf("Final worktree SHA after reset: %s", finalSHA)
	
	if finalSHA != newSHA {
		log.WarningLog.Printf("SHA mismatch after sync! Expected: %s, Got: %s", newSHA, finalSHA)
		return fmt.Errorf("failed to sync changes: SHA mismatch after reset")
	}
	
	log.InfoLog.Printf("Successfully synced rebase from clone to worktree locally")
	
	// Optionally, push to remote origin if the user wants to
	log.InfoLog.Printf("Changes synced locally. To push to remote, use 'git push --force-with-lease origin %s'", g.branchName)
	
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

// GetCurrentCommitSHA returns the current commit SHA
func (g *GitWorktree) GetCurrentCommitSHA() (string, error) {
	sha, err := g.runGitCommand(g.worktreePath, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("failed to get current commit SHA: %w", err)
	}
	return strings.TrimSpace(sha), nil
}

// FetchBranch fetches a specific branch from remote
func (g *GitWorktree) FetchBranch(branchName string) (string, error) {
	output, err := g.runGitCommand(g.worktreePath, "fetch", "origin", branchName)
	if err != nil {
		return "", err
	}
	return output, nil
}

// GetRemoteBranchSHA gets the SHA of a branch on the remote
func (g *GitWorktree) GetRemoteBranchSHA(branchName string) (string, error) {
	sha, err := g.runGitCommand(g.worktreePath, "rev-parse", fmt.Sprintf("origin/%s", branchName))
	if err != nil {
		return "", fmt.Errorf("failed to get remote branch SHA: %w", err)
	}
	return strings.TrimSpace(sha), nil
}

// ResetToRemote resets the current branch to match the remote
func (g *GitWorktree) ResetToRemote(branchName string) error {
	// First ensure we're on the right branch
	currentBranch, err := g.runGitCommand(g.worktreePath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}
	currentBranch = strings.TrimSpace(currentBranch)
	
	if currentBranch != branchName {
		if _, err := g.runGitCommand(g.worktreePath, "checkout", branchName); err != nil {
			return fmt.Errorf("failed to checkout branch %s: %w", branchName, err)
		}
	}
	
	// Reset to remote
	if _, err := g.runGitCommand(g.worktreePath, "reset", "--hard", fmt.Sprintf("origin/%s", branchName)); err != nil {
		return fmt.Errorf("failed to reset to remote: %w", err)
	}
	
	return nil
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

// GitFileStatus represents the status of a file in git
type GitFileStatus struct {
	Path   string
	Status string // M=Modified, A=Added, D=Deleted, R=Renamed, C=Copied
}

// GetChangedFilesForBranch gets all files changed in the current branch compared to main branch
func (g *GitWorktree) GetChangedFilesForBranch() ([]GitFileStatus, error) {
	// Determine the main branch name
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
			}
		}
	}

	// Get the merge base between current branch and main
	mergeBase, err := g.runGitCommand(g.worktreePath, "merge-base", fmt.Sprintf("origin/%s", mainBranch), "HEAD")
	if err != nil {
		return nil, fmt.Errorf("failed to find merge base: %w", err)
	}
	mergeBase = strings.TrimSpace(mergeBase)

	// Get changed files since merge base
	diffOutput, err := g.runGitCommand(g.worktreePath, "diff", "--name-status", mergeBase)
	if err != nil {
		return nil, fmt.Errorf("failed to get changed files: %w", err)
	}

	files := parseDiffNameStatus(diffOutput)

	// Sort files by status first, then by path for consistent ordering
	sortGitFileStatus(files)

	return files, nil
}

// parseDiffNameStatus parses the output of 'git diff --name-status'
func parseDiffNameStatus(diffOutput string) []GitFileStatus {
	var files []GitFileStatus
	lines := strings.Split(strings.TrimSpace(diffOutput), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		// Parse the status line format: "M\tfile.go" or "A\tfile.go"
		parts := strings.Split(line, "\t")
		if len(parts) >= 2 {
			files = append(files, GitFileStatus{
				Path:   parts[1],
				Status: parts[0],
			})
		}
	}
	return files
}

// sortGitFileStatus sorts a slice of GitFileStatus by status first, then by path
func sortGitFileStatus(files []GitFileStatus) {
	sort.Slice(files, func(i, j int) bool {
		if files[i].Status != files[j].Status {
			return files[i].Status < files[j].Status
		}
		return files[i].Path < files[j].Path
	})
}

// GetAllBookmarkCommits returns all bookmark commit SHAs in chronological order (oldest first)
func (g *GitWorktree) GetAllBookmarkCommits() ([]string, error) {
	// Get all bookmark commits on the current branch, in chronological order
	output, err := g.runGitCommand(g.worktreePath, "log", "--reverse", "--oneline", "--grep=^\\[BOOKMARK\\]", "--format=%H")
	if err != nil {
		// If no bookmarks found, return empty slice
		if strings.Contains(err.Error(), "does not have any commits") {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to get bookmark commits: %w", err)
	}

	if strings.TrimSpace(output) == "" {
		return []string{}, nil
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	var bookmarks []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			bookmarks = append(bookmarks, line)
		}
	}

	return bookmarks, nil
}

// GetChangedFilesBetweenCommits gets files changed between two commits
func (g *GitWorktree) GetChangedFilesBetweenCommits(fromCommit, toCommit string) ([]GitFileStatus, error) {
	var args []string
	if fromCommit == "" {
		// If no from commit, show all changes up to toCommit
		args = []string{"diff", "--name-status", fmt.Sprintf("%s^", toCommit), toCommit}
	} else {
		// Show changes between fromCommit and toCommit
		args = []string{"diff", "--name-status", fromCommit, toCommit}
	}

	output, err := g.runGitCommand(g.worktreePath, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get changed files between commits: %w", err)
	}

	files := parseDiffNameStatus(output)

	// Sort files by status first, then by path for consistent ordering
	sortGitFileStatus(files)

	return files, nil
}

// GetCommitMessage returns the commit message for a given SHA
func (g *GitWorktree) GetCommitMessage(commitSHA string) (string, error) {
	output, err := g.runGitCommand(g.worktreePath, "log", "--format=%s", "-n", "1", commitSHA)
	if err != nil {
		return "", fmt.Errorf("failed to get commit message: %w", err)
	}
	return strings.TrimSpace(output), nil
}

// GetChangedFilesSinceCommit gets all files changed since a specific commit (including uncommitted changes)
func (g *GitWorktree) GetChangedFilesSinceCommit(fromCommit string) ([]GitFileStatus, error) {
	// Get changes from the commit to HEAD
	output, err := g.runGitCommand(g.worktreePath, "diff", "--name-status", fromCommit, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("failed to get changed files since commit: %w", err)
	}

	files := parseDiffNameStatus(output)

	// Also get uncommitted changes (working directory + staged)
	uncommittedOutput, err := g.runGitCommand(g.worktreePath, "status", "--porcelain")
	if err == nil && strings.TrimSpace(uncommittedOutput) != "" {
		// Create a map to track existing files for O(1) lookup
		existingFiles := make(map[string]struct{})
		for _, f := range files {
			existingFiles[f.Path] = struct{}{}
		}
		
		uncommittedLines := strings.Split(strings.TrimSpace(uncommittedOutput), "\n")
		for _, line := range uncommittedLines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			
			// Parse porcelain format: "MM file.go" or " M file.go" or "A  file.go"
			if len(line) >= 3 {
				statusChars := line[:2]
				
				// Handle renamed files specially
				var filePath string
				if statusChars[0] == 'R' || statusChars[1] == 'R' {
					// Handle renamed files, which have the format "R  source -> destination"
					if parts := strings.Split(strings.TrimSpace(line[2:]), " -> "); len(parts) == 2 {
						filePath = parts[1] // Use destination path
					} else {
						continue // Skip malformed line
					}
				} else {
					filePath = strings.TrimSpace(line[2:])
				}
				
				// Convert porcelain status to diff status
				var status string
				switch {
				case strings.Contains(statusChars, "A"):
					status = "A" // Added
				case strings.Contains(statusChars, "D"):
					status = "D" // Deleted
				case strings.Contains(statusChars, "M"):
					status = "M" // Modified
				case strings.Contains(statusChars, "R"):
					status = "R" // Renamed
				case strings.Contains(statusChars, "C"):
					status = "C" // Copied
				case strings.Contains(statusChars, "?"):
					status = "A" // Untracked files as added
				default:
					status = "M" // Default to modified
				}
				
				// Check if this file is already in our list (avoid duplicates)
				if _, found := existingFiles[filePath]; !found {
					files = append(files, GitFileStatus{
						Path:   filePath,
						Status: status,
					})
					existingFiles[filePath] = struct{}{}
				}
			}
		}
	}

	// Sort files by status first, then by path for consistent ordering
	sortGitFileStatus(files)

	return files, nil
}
