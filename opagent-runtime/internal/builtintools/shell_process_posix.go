//go:build !windows

package builtintools

import (
	"os/exec"
	"syscall"
)

func ShellToolDescription() string {
	return "Execute a command using the host default POSIX shell: sh -c <command>. The command argument must use POSIX sh syntax."
}

func defaultShellInvocation(command string) shellInvocation {
	return shellInvocation{
		Executable:  "sh",
		Args:        []string{"-c", command},
		DisplayName: "sh",
	}
}

func prepareShellCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killProcessTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	_ = cmd.Process.Kill()
}
