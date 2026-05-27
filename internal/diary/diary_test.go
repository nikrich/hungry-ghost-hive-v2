package diary

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRead_ParsesEntries(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "diary.log")
	content := "2026-05-27T14:00:00Z\tmanager\ttick-start\n" +
		"2026-05-27T14:00:01Z\tmanager\tspawn\tagent=abc123 role=junior\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Read(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if got[0].Actor != "manager" || got[0].Event != "tick-start" {
		t.Errorf("entry 0: got %+v", got[0])
	}
	if got[1].Detail != "agent=abc123 role=junior" {
		t.Errorf("entry 1 Detail: got %q", got[1].Detail)
	}
}

func TestRead_ReturnsEmptyOnMissingFile(t *testing.T) {
	got, err := Read("/nonexistent/path/diary.log")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d, want 0", len(got))
	}
}

func TestTail_ReturnsLastN(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "diary.log")
	content := ""
	for i := 0; i < 10; i++ {
		content += "2026-05-27T14:00:00Z\tmanager\ttick-start\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Tail(path, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("got %d, want 3", len(got))
	}
}
