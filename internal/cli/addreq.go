package cli

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RunAddReq writes a requirement file to <workspaceRoot>/.hive/inbox/.
func RunAddReq(workspaceRoot, text string) error {
	inbox := filepath.Join(workspaceRoot, ".hive", "inbox")
	if info, err := os.Stat(inbox); err != nil || !info.IsDir() {
		return fmt.Errorf("inbox not found at %s: run `hive init` first", inbox)
	}

	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return err
	}
	name := fmt.Sprintf("req-%d-%s.txt", time.Now().Unix(), hex.EncodeToString(suffix))

	if !endsWithNewline(text) {
		text += "\n"
	}
	return os.WriteFile(filepath.Join(inbox, name), []byte(text), 0o644)
}

func endsWithNewline(s string) bool {
	return len(s) > 0 && s[len(s)-1] == '\n'
}
