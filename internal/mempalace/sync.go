// Package mempalace bridges hive's filesystem-based drawer reads with
// mempalace's ChromaDB-backed drawer storage.
//
// mempalace.add_drawer / update_drawer / list_drawers all read and write
// from ChromaDB documents; nothing materializes drawers as .md files on
// disk. Hive's internal/drawers.List scans <wingRoot>/rooms/<room>/*.md
// from the filesystem. This package closes the gap by shelling out to a
// Python one-liner against the workspace-local ChromaDB.
//
// Phase 1.5 bridge: DumpToFilesystem snapshots every drawer to disk before
// RunMerge reads them; PushDrawer pushes the merge-flipped .md back to
// ChromaDB so the next manager tick observes status=merged.
package mempalace

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
)

// pythonBin is the venv-pinned interpreter that hive init's mcp.json
// already targets. Keeping it hardcoded here mirrors that pinning so
// drift between the two sites surfaces quickly.
const pythonBin = "/Users/jannik/.agentflow/.venv/bin/python3"

// chromaPath returns <workspaceRoot>/.hive/memory/index/chroma — the
// path mempalace_gateway sets MEMPALACE_PALACE_PATH to when the workspace
// mcp.json (Phase 1.5) is in effect.
func chromaPath(workspaceRoot string) string {
	return filepath.Join(workspaceRoot, ".hive", "memory", "index", "chroma")
}

// wingRoot returns <workspaceRoot>/.hive/memory/wings/hive.
func wingRoot(workspaceRoot string) string {
	return filepath.Join(workspaceRoot, ".hive", "memory", "wings", "hive")
}

// hasChroma reports whether the workspace has a populated ChromaDB.
// When absent (e.g. inside unit tests with pre-seeded .md files),
// the sync functions become no-ops so tests stay green.
func hasChroma(workspaceRoot string) bool {
	info, err := os.Stat(filepath.Join(chromaPath(workspaceRoot), "chroma.sqlite3"))
	return err == nil && !info.IsDir()
}

// drawerRecord is the wire shape returned by the Python dump script.
type drawerRecord struct {
	DrawerID string `json:"drawer_id"`
	Wing     string `json:"wing"`
	Room     string `json:"room"`
	Content  string `json:"content"`
}

// DumpToFilesystem mirrors every drawer in the workspace ChromaDB into
// <wingRoot>/rooms/<room>/<drawer_id>.md. Existing .md files in those
// rooms are cleared first so stale entries from a prior run don't leak.
//
// Returns nil (no-op) when no ChromaDB is present at the workspace path.
func DumpToFilesystem(workspaceRoot string) error {
	if !hasChroma(workspaceRoot) {
		return nil
	}

	records, err := runDump(workspaceRoot)
	if err != nil {
		return fmt.Errorf("dump chroma: %w", err)
	}

	roomsByName := map[string]struct{}{}
	for _, r := range records {
		if r.Wing != "hive" || r.Room == "" || r.DrawerID == "" {
			continue
		}
		roomsByName[r.Room] = struct{}{}
	}

	// Clear .md files in every room we're about to repopulate. Other
	// files (none expected in practice) are left untouched.
	for room := range roomsByName {
		dir := filepath.Join(wingRoot(workspaceRoot), "rooms", room)
		if err := clearMarkdownFiles(dir); err != nil {
			return fmt.Errorf("clear %s: %w", dir, err)
		}
	}

	for _, r := range records {
		if r.Wing != "hive" || r.Room == "" || r.DrawerID == "" {
			continue
		}
		dir := filepath.Join(wingRoot(workspaceRoot), "rooms", r.Room)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
		path := filepath.Join(dir, r.DrawerID+".md")
		if err := os.WriteFile(path, []byte(r.Content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

// PushDrawer overwrites the ChromaDB document for drawerID with content.
// Returns nil when ChromaDB is absent so unit tests with seeded .md files
// keep passing.
func PushDrawer(workspaceRoot, drawerID, content string) error {
	if !hasChroma(workspaceRoot) {
		return nil
	}
	return runUpdate(workspaceRoot, drawerID, content)
}

// DrawerIDFromPath extracts the chroma drawer_id from a .md path written
// by DumpToFilesystem. Path shape: <wingRoot>/rooms/<room>/<id>.md.
// Returns "" when the path doesn't fit (e.g. a hand-written test drawer
// like story-quickstart.md without a chroma id).
func DrawerIDFromPath(path string) string {
	base := filepath.Base(path)
	const suffix = ".md"
	if len(base) <= len(suffix) || base[len(base)-len(suffix):] != suffix {
		return ""
	}
	id := base[:len(base)-len(suffix)]
	// Chroma IDs from mempalace look like drawer_<wing>_<room>_<hash>.
	if len(id) < 8 || id[:7] != "drawer_" {
		return ""
	}
	return id
}

func clearMarkdownFiles(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if len(name) > 3 && name[len(name)-3:] == ".md" {
			if err := os.Remove(filepath.Join(dir, name)); err != nil {
				return err
			}
		}
	}
	return nil
}

const dumpScript = `
import json, os, sys
os.environ["MEMPALACE_PALACE_PATH"] = os.environ["WORKSPACE_CHROMA"]
from mempalace.mcp_server import TOOLS
listed = TOOLS["mempalace_list_drawers"]["handler"](wing="hive", limit=1000)
out = []
for d in listed.get("drawers", []):
    full = TOOLS["mempalace_get_drawer"]["handler"](drawer_id=d["drawer_id"])
    if full.get("error"):
        continue
    out.append({
        "drawer_id": full["drawer_id"],
        "wing": full.get("wing", ""),
        "room": full.get("room", ""),
        "content": full.get("content", ""),
    })
json.dump(out, sys.stdout)
`

const updateScript = `
import json, os, sys
os.environ["MEMPALACE_PALACE_PATH"] = os.environ["WORKSPACE_CHROMA"]
from mempalace.mcp_server import TOOLS
drawer_id = sys.argv[1]
content = sys.stdin.read()
result = TOOLS["mempalace_update_drawer"]["handler"](drawer_id=drawer_id, content=content)
json.dump(result, sys.stdout)
`

func runDump(workspaceRoot string) ([]drawerRecord, error) {
	cmd := exec.Command(pythonBin, "-c", dumpScript)
	cmd.Env = append(os.Environ(), "WORKSPACE_CHROMA="+chromaPath(workspaceRoot))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, stderr.String())
	}
	var out []drawerRecord
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return nil, fmt.Errorf("decode dump output: %w", err)
	}
	return out, nil
}

func runUpdate(workspaceRoot, drawerID, content string) error {
	cmd := exec.Command(pythonBin, "-c", updateScript, drawerID)
	cmd.Env = append(os.Environ(), "WORKSPACE_CHROMA="+chromaPath(workspaceRoot))
	cmd.Stdin = bytes.NewReader([]byte(content))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, stderr.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return fmt.Errorf("decode update output: %w (raw=%q)", err, stdout.String())
	}
	if errStr, ok := resp["error"].(string); ok && errStr != "" {
		return fmt.Errorf("mempalace update_drawer rejected: %s", errStr)
	}
	return nil
}
