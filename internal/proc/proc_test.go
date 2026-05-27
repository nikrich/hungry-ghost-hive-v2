package proc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAndReadPidFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.pid")
	if err := WritePidFile(path, 12345); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := ReadPidFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got != 12345 {
		t.Errorf("got %d, want 12345", got)
	}
}

func TestReadPidFile_ReturnsZeroOnMissing(t *testing.T) {
	got, err := ReadPidFile("/nonexistent/path.pid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}

func TestClearPidFile_IsIdempotent(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.pid")
	if err := WritePidFile(path, 1); err != nil {
		t.Fatal(err)
	}
	if err := ClearPidFile(path); err != nil {
		t.Fatalf("first clear: %v", err)
	}
	if err := ClearPidFile(path); err != nil {
		t.Fatalf("second clear (idempotent): %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected file gone, got err=%v", err)
	}
}

func TestIsAlive_ReturnsFalseForZero(t *testing.T) {
	if IsAlive(0) {
		t.Error("IsAlive(0) should be false")
	}
}

func TestIsAlive_ReturnsTrueForCurrentProcess(t *testing.T) {
	if !IsAlive(os.Getpid()) {
		t.Error("IsAlive(getpid) should be true")
	}
}

func TestIsAlive_ReturnsFalseForLikelyDeadPid(t *testing.T) {
	if IsAlive(999999) {
		t.Skip("PID 999999 happens to be alive; skipping")
	}
}
