//go:build windows

package ws

import (
	"os/exec"
	"syscall"
)

func resolveCommandShellInvocation(command string) commandShellInvocation {
	return commandShellInvocation{
		Executable: "powershell.exe",
		Args:       []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", command},
	}
}

func prepareCommandProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

func killCommandProcessTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
