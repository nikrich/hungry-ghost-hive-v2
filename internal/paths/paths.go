// Package paths discovers workspace and mempalace storage roots.
package paths

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrNoWorkspace is returned when no .hive directory is found in the ancestry of the given path.
var ErrNoWorkspace = errors.New("no .hive directory found in ancestry; not inside a hive workspace")

// ErrMempalaceRootUnset is returned when MEMPALACE_ROOT env var is not set.
var ErrMempalaceRootUnset = errors.New("MEMPALACE_ROOT environment variable not set")

// FindWorkspaceRoot walks up from start until it finds a directory containing .hive/.
// Returns the workspace root or ErrNoWorkspace.
func FindWorkspaceRoot(start string) (string, error) {
	cur, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if info, err := os.Stat(filepath.Join(cur, ".hive")); err == nil && info.IsDir() {
			return cur, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", ErrNoWorkspace
		}
		cur = parent
	}
}

// MempalaceRoot returns the path from $MEMPALACE_ROOT.
func MempalaceRoot() (string, error) {
	v := os.Getenv("MEMPALACE_ROOT")
	if v == "" {
		return "", ErrMempalaceRootUnset
	}
	return v, nil
}

// HiveDir returns <workspaceRoot>/.hive.
func HiveDir(workspaceRoot string) string {
	return filepath.Join(workspaceRoot, ".hive")
}

// ClaudeDir returns <workspaceRoot>/.claude.
func ClaudeDir(workspaceRoot string) string {
	return filepath.Join(workspaceRoot, ".claude")
}

// WingDir returns the on-disk directory for a workspace's mempalace wing.
func WingDir(mempalaceRoot, wingSlug string) string {
	return filepath.Join(mempalaceRoot, "wings", "hive-"+wingSlug)
}
