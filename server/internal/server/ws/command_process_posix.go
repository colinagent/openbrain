//go:build !windows

package ws

import (
	"os/exec"
	"syscall"
)

func resolveCommandShellInvocation(command string) commandShellInvocation {
	return commandShellInvocation{
		Executable: "sh",
		Args:       []string{"-c", command},
	}
}

func prepareCommandProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killCommandProcessTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	_ = cmd.Process.Kill()
}
