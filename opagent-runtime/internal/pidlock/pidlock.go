package pidlock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gofrs/flock"
)

const (
	lockFileName = "opagent-runtime.lock"
	pidFileName  = "opagent-runtime.pid"
)

// Manager manages the PID file and lock file to ensure a single instance runs.
type Manager struct {
	lockPath string
	pidPath  string
	flock    *flock.Flock
}

// New creates a Manager for the given run directory.
func New(runDir string) *Manager {
	lockPath := filepath.Join(runDir, lockFileName)
	pidPath := filepath.Join(runDir, pidFileName)
	return &Manager{
		lockPath: lockPath,
		pidPath:  pidPath,
		flock:    flock.New(lockPath),
	}
}

// Acquire tries to acquire the instance lock and write the current PID.
func (m *Manager) Acquire() error {
	if m == nil || m.flock == nil {
		return errors.New("pid lock manager is not initialized")
	}
	if err := os.MkdirAll(filepath.Dir(m.lockPath), 0o755); err != nil {
		return fmt.Errorf("create run dir: %w", err)
	}

	locked, err := m.flock.TryLock()
	if err != nil {
		return fmt.Errorf("try lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("another instance is running (lock held)")
	}

	return m.writePID()
}

// Release removes the PID file and releases the lock.
func (m *Manager) Release() error {
	if m == nil {
		return nil
	}
	_ = os.Remove(m.pidPath)
	if m.flock == nil {
		return nil
	}
	if err := m.flock.Unlock(); err != nil {
		return fmt.Errorf("unlock: %w", err)
	}
	return nil
}

func (m *Manager) writePID() error {
	pid := os.Getpid()
	if err := os.WriteFile(m.pidPath, []byte(strconv.Itoa(pid)), 0o644); err != nil {
		return fmt.Errorf("write pid: %w", err)
	}
	return nil
}
