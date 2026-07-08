//go:build !unix && !windows

package main

import (
	"os"
	"os/exec"
)

func prepareRuntimeCommand(cmd *exec.Cmd) {}

func terminatePID(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}
