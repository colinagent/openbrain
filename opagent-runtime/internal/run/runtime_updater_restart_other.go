//go:build !unix

package run

import "fmt"

func restartRuntimeProcess(executablePath string, args []string, env []string) error {
	return fmt.Errorf("runtime updater restart is not supported on this platform")
}
