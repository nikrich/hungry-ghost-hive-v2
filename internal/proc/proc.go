// Package proc handles PID files and process lifecycle.
package proc

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// WritePidFile writes pid to path atomically (write then rename).
func WritePidFile(path string, pid int) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strconv.Itoa(pid)), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ReadPidFile returns the PID stored at path, or 0 if the file doesn't exist.
func ReadPidFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// ClearPidFile removes the PID file. Missing file is not an error.
func ClearPidFile(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// IsAlive reports whether the process with the given PID exists.
// On Unix this uses signal 0 which checks existence without sending anything.
func IsAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

// Terminate sends SIGTERM to pid, waits up to grace, then sends SIGKILL.
// Returns nil on clean exit, error if SIGKILL was needed or if the process never appeared dead.
func Terminate(pid int, grace time.Duration) error {
	if !IsAlive(pid) {
		return nil
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("SIGTERM %d: %w", pid, err)
	}
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if !IsAlive(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		return fmt.Errorf("SIGKILL %d: %w", pid, err)
	}
	return fmt.Errorf("pid %d required SIGKILL", pid)
}
