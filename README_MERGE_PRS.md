# Merge Multiple PRs Feature

This feature allows you to select and merge multiple pull requests into a single branch and create a new PR.

## Usage

1. Select an instance in claude-squad
2. Press `M` to open the PR selector
3. Use arrow keys to navigate through open PRs
4. Press `Space` to select/deselect PRs
5. Press `Enter` to merge selected PRs
6. A new branch will be created with all merged changes
7. A new PR will be automatically created

## Features

- Lists all open PRs in the repository
- Allows multi-selection of PRs
- Cherry-picks commits from each selected PR
- Handles merge conflicts gracefully (skips conflicting PRs)
- Creates a consolidated PR with all successfully merged changes
- Automatically cleans up the temporary merge worktree

## Key Bindings

- `M` - Open merge PRs dialog
- `Space` - Toggle PR selection
- `Enter` - Confirm and merge selected PRs
- `q` or `Esc` - Cancel

## Error Handling

- If a PR fails to fetch, it will be skipped
- If a PR has merge conflicts, it will be skipped
- Only successfully merged PRs will be included in the final PR
- If no PRs can be merged successfully, the operation will fail

## Implementation Details

The feature:
1. Creates a new worktree from the main branch
2. Fetches and cherry-picks commits from each selected PR
3. Creates a single commit with all changes
4. Pushes the new branch
5. Creates a new PR using GitHub CLI