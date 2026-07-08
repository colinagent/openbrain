//go:build unix

package run

import "syscall"

func restartRuntimeProcess(executablePath string, args []string, env []string) error {
	return syscall.Exec(executablePath, args, env)
}
