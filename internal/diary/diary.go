// Package diary reads the mempalace event log.
package diary

import (
	"bufio"
	"errors"
	"os"
	"strings"
)

// Entry is one event-log line.
type Entry struct {
	Timestamp string // ISO 8601
	Actor     string // "manager", "worker", "watchdog"
	Event     string // short event name
	Detail    string // free-form rest of line
}

// Read returns all entries from the given diary file.
// Missing file returns an empty slice (not an error).
func Read(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []Entry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if e, ok := parseLine(scanner.Text()); ok {
			out = append(out, e)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Tail returns the last n entries.
func Tail(path string, n int) ([]Entry, error) {
	all, err := Read(path)
	if err != nil {
		return nil, err
	}
	if len(all) <= n {
		return all, nil
	}
	return all[len(all)-n:], nil
}

func parseLine(line string) (Entry, bool) {
	parts := strings.SplitN(line, "\t", 4)
	if len(parts) < 3 {
		return Entry{}, false
	}
	e := Entry{
		Timestamp: parts[0],
		Actor:     parts[1],
		Event:     parts[2],
	}
	if len(parts) == 4 {
		e.Detail = parts[3]
	}
	return e, true
}
