// Package drawers reads mempalace drawers from disk.
package drawers

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Drawer is the parsed view of a single mempalace drawer file.
// Captures the frontmatter fields hive cares about; the full body is kept for callers that need more.
type Drawer struct {
	Title       string `yaml:"title"`
	Type        string `yaml:"type"`
	Status      string `yaml:"status"`
	Points      int    `yaml:"points"`
	Team        string `yaml:"team"`
	AssignedTo  string `yaml:"assigned_to"`
	Role        string `yaml:"role"`
	Story       string `yaml:"story"`
	PRURL       string `yaml:"pr_url"`
	RetryCount  int    `yaml:"retry_count"`
	CreatedAt   string `yaml:"created_at"`
	UpdatedAt   string `yaml:"updated_at"`

	// Phase 2.A — decomposition / dependency / criteria fields
	DependsOn          []string `yaml:"depends_on"`
	AcceptanceCriteria []string `yaml:"acceptance_criteria"`
	ParentRequirement  string   `yaml:"parent_requirement"`
	DecomposedInto     []string `yaml:"decomposed_into"`
	CurrentRequirement string   `yaml:"current_requirement"`

	// Phase 2.D — feature branch + merge timestamp
	FeatureBranch string `yaml:"feature_branch"`
	MergedAt      string `yaml:"merged_at"`

	Path string `yaml:"-"` // filesystem path, set by List
	Body string `yaml:"-"` // markdown body after the frontmatter
}

// List walks <wingRoot>/rooms/<room>/*.md and returns parsed drawers.
// Missing room directory returns an empty slice (not an error).
func List(wingRoot, room string) ([]Drawer, error) {
	dir := filepath.Join(wingRoot, "rooms", room)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read room dir: %w", err)
	}

	var out []Drawer
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		d, err := parseFile(path)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		out = append(out, d)
	}
	return out, nil
}

// FilterByStatus returns drawers whose Status equals the given value.
func FilterByStatus(in []Drawer, status string) []Drawer {
	var out []Drawer
	for _, d := range in {
		if d.Status == status {
			out = append(out, d)
		}
	}
	return out
}

// FilterByType returns drawers whose Type equals the given value.
func FilterByType(in []Drawer, drawerType string) []Drawer {
	var out []Drawer
	for _, d := range in {
		if d.Type == drawerType {
			out = append(out, d)
		}
	}
	return out
}

func parseFile(path string) (Drawer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Drawer{}, err
	}
	d, err := parse(data)
	if err != nil {
		return Drawer{}, err
	}
	d.Path = path
	return d, nil
}

// parse extracts YAML frontmatter (between leading `---` markers) and the body.
func parse(data []byte) (Drawer, error) {
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return Drawer{}, errors.New("missing leading frontmatter marker")
	}
	rest := data[4:]
	end := bytes.Index(rest, []byte("\n---\n"))
	if end < 0 {
		return Drawer{}, errors.New("missing trailing frontmatter marker")
	}
	front := rest[:end]
	body := rest[end+5:]

	var d Drawer
	if err := yaml.Unmarshal(front, &d); err != nil {
		return Drawer{}, fmt.Errorf("yaml: %w", err)
	}
	d.Body = string(body)
	return d, nil
}
