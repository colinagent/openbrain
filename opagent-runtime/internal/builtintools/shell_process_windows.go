//go:build windows

package builtintools

import (
	"os/exec"
	"syscall"
)

func ShellToolDescription() string {
	return "Execute a command using Windows PowerShell: powershell.exe -NoProfile -ExecutionPolicy Bypass -Command <command>. The command argument must use PowerShell syntax."
}

func defaultShellInvocation(command string) shellInvocation {
	return shellInvocation{
		Executable:  "powershell.exe",
		Args:        []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", command},
		DisplayName: "powershell.exe",
	}
}

func prepareShellCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

func killProcessTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
