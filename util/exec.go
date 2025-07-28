package util

import (
	"claude-squad/log"
	"os/exec"
)

// Command creates a new exec.Cmd and logs it
func Command(source string, name string, arg ...string) *exec.Cmd {
	cmd := exec.Command(name, arg...)
	log.LogExecCommand(cmd, source)
	return cmd
}