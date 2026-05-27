package drawers

import (
	"os"
	"path/filepath"
	"testing"
)

func writeDrawer(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestList_ParsesFrontmatter(t *testing.T) {
	tmp := t.TempDir()
	writeDrawer(t, filepath.Join(tmp, "rooms", "stories", "STORY-001.md"),
		"---\ntitle: STORY-001\ntype: story\nstatus: pending\n---\n\nBody\n")

	got, err := List(tmp, "stories")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d drawers, want 1", len(got))
	}
	if got[0].Title != "STORY-001" {
		t.Errorf("Title: got %q, want STORY-001", got[0].Title)
	}
	if got[0].Type != "story" {
		t.Errorf("Type: got %q, want story", got[0].Type)
	}
	if got[0].Status != "pending" {
		t.Errorf("Status: got %q, want pending", got[0].Status)
	}
}

func TestList_SkipsNonMarkdown(t *testing.T) {
	tmp := t.TempDir()
	writeDrawer(t, filepath.Join(tmp, "rooms", "stories", "STORY-001.md"),
		"---\ntitle: t\ntype: story\n---\nBody\n")
	writeDrawer(t, filepath.Join(tmp, "rooms", "stories", "notes.txt"), "ignored\n")

	got, err := List(tmp, "stories")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d, want 1 (only .md)", len(got))
	}
}

func TestList_ReturnsEmptyOnMissingRoom(t *testing.T) {
	tmp := t.TempDir()
	got, err := List(tmp, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d, want 0", len(got))
	}
}

func TestFilterByStatus(t *testing.T) {
	all := []Drawer{
		{Title: "a", Status: "pending"},
		{Title: "b", Status: "in-progress"},
		{Title: "c", Status: "pending"},
	}
	got := FilterByStatus(all, "pending")
	if len(got) != 2 {
		t.Errorf("got %d, want 2", len(got))
	}
}
