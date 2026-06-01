package mempalace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasChroma_FalseWhenAbsent(t *testing.T) {
	ws := t.TempDir()
	if hasChroma(ws) {
		t.Fatal("hasChroma should be false for an empty workspace")
	}
}

func TestDumpToFilesystem_NoOpWhenChromaAbsent(t *testing.T) {
	ws := t.TempDir()
	if err := DumpToFilesystem(ws); err != nil {
		t.Fatalf("DumpToFilesystem: %v", err)
	}
	// Nothing should have been created.
	if _, err := os.Stat(filepath.Join(ws, ".hive")); !os.IsNotExist(err) {
		t.Fatalf(".hive should not exist after no-op dump: %v", err)
	}
}

func TestPushDrawer_NoOpWhenChromaAbsent(t *testing.T) {
	ws := t.TempDir()
	if err := PushDrawer(ws, "drawer_hive_stories_abc", "---\nstatus: merged\n---\n"); err != nil {
		t.Fatalf("PushDrawer no-op: %v", err)
	}
}

func TestDrawerIDFromPath(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/x/rooms/stories/drawer_hive_stories_abc123.md", "drawer_hive_stories_abc123"},
		{"rooms/stories/drawer_hive_stories_xyz.md", "drawer_hive_stories_xyz"},
		{"rooms/stories/story-quickstart.md", ""}, // hand-written test drawer
		{"rooms/stories/notes.txt", ""},           // not .md
		{".md", ""},                               // pathological — no prefix
	}
	for _, c := range cases {
		got := DrawerIDFromPath(c.path)
		if got != c.want {
			t.Errorf("DrawerIDFromPath(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}
