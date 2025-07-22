# Claude Squad Configuration Example

This repository demonstrates the per-repository configuration system for claude-squad.

## Configuration

[claude-squad]
ide_command: code
diff_command: code --diff

## Usage

- Press `w` to open the current instance in your configured IDE (Visual Studio Code in this case)
- Press `i` in diff view to open the current file in your IDE 
- Press `x` in diff view to open the current file in your external diff tool

## Configuration Options

### Global Configuration
Edit `~/.claude-squad/config.json`:
```json
{
  "default_ide_command": "webstorm",
  "default_diff_command": "meld"
}
```

### Per-Repository Configuration
Option 1: Add to your `CLAUDE.md` file:
```markdown
[claude-squad]
ide_command: code
diff_command: code --diff
```

Option 2: Create `.claude-squad/config.json` in your repository:
```json
{
  "ide_command": "code",
  "diff_command": "meld"
}
```

Per-repository configuration takes precedence over global configuration.