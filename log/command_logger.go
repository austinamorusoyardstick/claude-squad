package log

import (
	"os/exec"
	"sync"
)

// CommandLogger is a global interface for logging command executions
type CommandLogger interface {
	LogCommand(cmd *exec.Cmd, source string)
}

var (
	commandLogger CommandLogger
	loggerMu      sync.RWMutex
)

// SetCommandLogger sets the global command logger
func SetCommandLogger(logger CommandLogger) {
	loggerMu.Lock()
	defer loggerMu.Unlock()
	commandLogger = logger
}

// LogExecCommand logs an exec.Command call
func LogExecCommand(cmd *exec.Cmd, source string) {
	loggerMu.RLock()
	logger := commandLogger
	loggerMu.RUnlock()

	if logger != nil {
		logger.LogCommand(cmd, source)
	}
}