package app

import (
	"claude-squad/ui"
	"os/exec"
)

// CommandLoggerAdapter adapts the LogPane to the CommandLogger interface
type CommandLoggerAdapter struct {
	logPane *ui.LogPane
}

// NewCommandLoggerAdapter creates a new command logger adapter
func NewCommandLoggerAdapter(logPane *ui.LogPane) *CommandLoggerAdapter {
	return &CommandLoggerAdapter{
		logPane: logPane,
	}
}

// LogCommand implements the CommandLogger interface
func (a *CommandLoggerAdapter) LogCommand(cmd *exec.Cmd, source string) {
	if a.logPane != nil && cmd != nil {
		dir := ""
		if cmd.Dir != "" {
			dir = cmd.Dir
		}
		a.logPane.AddLog(cmd.Path, cmd.Args[1:], dir, source)
	}
}