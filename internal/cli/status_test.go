package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderStatus_IncludesStoryCounts(t *testing.T) {
	tmp := t.TempDir()
	wingRoot := filepath.Join(tmp, "wings", "hive-test")
	storyDir := filepath.Join(wingRoot, "rooms", "stories")
	if err := os.MkdirAll(storyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i, s := range []string{"pending", "pending", "merged"} {
		path := filepath.Join(storyDir, "STORY-00"+string(rune('1'+i))+".md")
		body := "---\ntitle: t\ntype: story\nstatus: " + s + "\n---\nbody\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var buf bytes.Buffer
	if err := RenderStatus(&buf, wingRoot); err != nil {
		t.Fatalf("RenderStatus: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "stories: 3") {
		t.Errorf("expected 'stories: 3' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "pending: 2") {
		t.Errorf("expected 'pending: 2' in output, got:\n%s", out)
	}
}
