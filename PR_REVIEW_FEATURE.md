# Pull Request Comment Review Feature

This feature allows you to review pull request comments and automatically process them with Claude.

## Features

- Fetch and display all PR comments (both review comments and issue comments)
- Interactive UI to review comments one by one
- Accept or deny individual comments
- Bulk accept/deny all comments
- Automatically send accepted comments to Claude for processing

## Usage

1. **Prerequisites**
   - The selected instance must have an active pull request for its branch
   - GitHub CLI (`gh`) must be installed and authenticated
   - The instance must be started (not paused)

2. **Access the PR Review Interface**
   - In claude-squad, select a started instance with a PR
   - Press `R` to open the PR review interface for that instance's worktree

3. **Review Comments**
   - Use `j/k` or arrow keys to navigate between comments
   - Press `a` to accept the current comment
   - Press `d` to deny the current comment
   - Press `A` to accept all comments
   - Press `D` to deny all comments
   - Press `Enter` to process accepted comments
   - Press `q` or `Esc` to cancel

4. **Processing**
   - Accepted comments are sent to Claude one by one
   - Each comment is formatted with context (author, file, line number)
   - Claude will attempt to address the review feedback

## Implementation Details

- **PR Comment Fetching**: Uses GitHub CLI to fetch both review comments and issue comments from the selected instance's worktree
- **Worktree Integration**: PR is fetched from the specific worktree path of the selected instance, not the current directory
- **UI Component**: Built with bubbletea framework for interactive terminal UI
- **Claude Integration**: Formats comments as prompts and sends them directly to the AI pane (Claude) in the active instance
- **AI Pane Communication**: Uses tmux to send prompts directly to pane 1 where Claude is running

## File Structure

- `session/git/pr_comments.go` - PR comment fetching and data structures
- `ui/pr_review.go` - Interactive UI component for reviewing comments
- `app/pr_processor.go` - Logic for processing accepted comments with Claude
- `keys/keys.go` - Key binding for PR review (R key)