package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nikrich/hungry-ghost-hive-v2/internal/drawers"
	"github.com/nikrich/hungry-ghost-hive-v2/internal/mempalace"
	"github.com/nikrich/hungry-ghost-hive-v2/internal/paths"
)

// MergeOptions is the input to RunMerge.
type MergeOptions struct {
	WorkspaceRoot string
	StoryTitle    string // exact-match against story drawer titles
	DryRun        bool
}

// RunMerge merges a worker's agent branch into its parent requirement's
// feature branch and flips the story drawer to merged.
//
// Operator-driven for now (Phase 2.D); Phase 2.C's QA role calls the same
// primitive. The story must be in status=review and must have a feature_branch
// set. Agent branch is reconstructed from the story's assigned_to + role.
func RunMerge(opts MergeOptions) error {
	// Phase 1.5 bridge: snapshot ChromaDB drawers to disk so findStoryByTitle
	// and lookupAgentRole see the live state written by manager/worker MCP
	// calls. No-op when the workspace ChromaDB is absent (unit tests).
	if err := mempalace.DumpToFilesystem(opts.WorkspaceRoot); err != nil {
		return fmt.Errorf("sync drawers from chroma: %w", err)
	}

	wingRoot := paths.WorkspaceWingDir(opts.WorkspaceRoot)

	story, err := findStoryByTitle(wingRoot, opts.StoryTitle)
	if err != nil {
		return err
	}
	if story.Status != "review" {
		return fmt.Errorf("story %q is %s, cannot merge (must be 'review')", story.Title, story.Status)
	}
	if story.FeatureBranch == "" {
		return fmt.Errorf("story %q has no feature_branch (legacy Phase 2.A shape); merge manually with git or re-decompose under Phase 2.D", story.Title)
	}
	if story.Team == "" {
		return fmt.Errorf("story %q has no team set", story.Title)
	}
	if story.AssignedTo == "" {
		return fmt.Errorf("story %q has no assigned_to set; cannot reconstruct agent branch", story.Title)
	}

	// Reconstruct agent branch from assigned_to + role looked up via agent-state drawer.
	role, err := lookupAgentRole(wingRoot, story.AssignedTo)
	if err != nil {
		return err
	}
	agentBranch := fmt.Sprintf("agent/%s--%s-%s", story.Team, role, story.AssignedTo)

	if opts.DryRun {
		fmt.Printf("DRY RUN — would run in repos/%s:\n", story.Team)
		fmt.Printf("  git fetch origin --quiet\n")
		fmt.Printf("  git checkout %s\n", story.FeatureBranch)
		fmt.Printf("  git pull origin %s --quiet\n", story.FeatureBranch)
		fmt.Printf("  git merge --no-ff --no-edit origin/%s\n", agentBranch)
		fmt.Printf("  git push origin %s\n", story.FeatureBranch)
		fmt.Printf("  git checkout -\n")
		fmt.Printf("Then flip story drawer %q → merged, merged_at=<now>\n", story.Title)
		return nil
	}

	if err := runGitMerge(opts.WorkspaceRoot, story.Team, story.FeatureBranch, agentBranch); err != nil {
		return fmt.Errorf("merge failed: %w. Resolve manually with:\n  git -C repos/%s checkout %s && git merge origin/%s && git push\nThen update the story drawer or re-run hive merge", err, story.Team, story.FeatureBranch, agentBranch)
	}

	if err := flipStoryMerged(story.Path); err != nil {
		return fmt.Errorf("merge succeeded but flipping drawer failed: %w (manually set status=merged in %s)", err, story.Path)
	}

	// Phase 1.5 bridge: push the flipped drawer back to ChromaDB so the
	// next manager tick observes status=merged via mempalace_list_drawers.
	// Skipped silently for hand-written drawer files in tests (DrawerID="").
	if drawerID := mempalace.DrawerIDFromPath(story.Path); drawerID != "" {
		flipped, readErr := os.ReadFile(story.Path)
		if readErr != nil {
			return fmt.Errorf("merge + drawer flip succeeded but re-reading drawer for chroma push failed: %w", readErr)
		}
		if err := mempalace.PushDrawer(opts.WorkspaceRoot, drawerID, string(flipped)); err != nil {
			return fmt.Errorf("merge + drawer flip succeeded but chroma push failed: %w (manager tick will still see status=review until you re-run hive merge)", err)
		}
	}

	fmt.Printf("Merged origin/%s into origin/%s\n", agentBranch, story.FeatureBranch)
	fmt.Printf("Story %q → merged\n", story.Title)
	fmt.Println("Next manager tick (~60s) will pick up dependent stories whose deps are now satisfied.")
	return nil
}

// findStoryByTitle scans <wingRoot>/rooms/stories for a drawer with the given title.
func findStoryByTitle(wingRoot, title string) (drawers.Drawer, error) {
	all, err := drawers.List(wingRoot, "stories")
	if err != nil {
		return drawers.Drawer{}, fmt.Errorf("list stories: %w", err)
	}
	for _, d := range all {
		if d.Title == title {
			return d, nil
		}
	}
	return drawers.Drawer{}, fmt.Errorf("no story drawer found with title %q", title)
}

// lookupAgentRole finds the agent-state drawer for the given agent id and returns its role.
func lookupAgentRole(wingRoot, agentID string) (string, error) {
	all, err := drawers.List(wingRoot, "agents")
	if err != nil {
		return "", fmt.Errorf("list agents: %w", err)
	}
	want := "agent-" + agentID
	for _, d := range all {
		if d.Title == want {
			if d.Role == "" {
				return "", fmt.Errorf("agent-state drawer %q has no role", want)
			}
			return d.Role, nil
		}
	}
	return "", fmt.Errorf("no agent-state drawer found for agent id %q (expected title %q)", agentID, want)
}

// runGitMerge fetches, checks out the feature branch, merges the agent branch with --no-ff,
// pushes, and restores the previous HEAD. On any error AFTER the checkout the previous HEAD
// is best-effort restored (fetch-failure returns early because HEAD has not moved yet).
func runGitMerge(workspaceRoot, team, featureBranch, agentBranch string) error {
	repoDir := filepath.Join(workspaceRoot, "repos", team)
	if _, err := os.Stat(repoDir); err != nil {
		return fmt.Errorf("team repo %s not found: %w", repoDir, err)
	}

	origHead, err := gitOutput(repoDir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return fmt.Errorf("rev-parse HEAD: %w", err)
	}
	origHead = strings.TrimSpace(origHead)
	if origHead == "" {
		origHead = "main"
	}

	restoreHEAD := func() {
		_ = gitRun(repoDir, "checkout", origHead)
	}

	if err := gitRun(repoDir, "fetch", "origin", "--quiet"); err != nil {
		return fmt.Errorf("git fetch: %w", err)
	}
	if err := gitRun(repoDir, "checkout", featureBranch); err != nil {
		return fmt.Errorf("git checkout %s: %w", featureBranch, err)
	}
	if err := gitRun(repoDir, "pull", "origin", featureBranch, "--quiet"); err != nil {
		restoreHEAD()
		return fmt.Errorf("git pull %s: %w", featureBranch, err)
	}
	if err := gitRun(repoDir, "merge", "--no-ff", "--no-edit", "origin/"+agentBranch); err != nil {
		_ = gitRun(repoDir, "merge", "--abort") // best effort; the original merge error is the one returned
		restoreHEAD()
		return fmt.Errorf("git merge origin/%s: %w", agentBranch, err)
	}
	if err := gitRun(repoDir, "push", "origin", featureBranch); err != nil {
		restoreHEAD()
		return fmt.Errorf("git push %s: %w", featureBranch, err)
	}
	restoreHEAD()
	return nil
}

// gitRun runs git in dir, discarding output.
func gitRun(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// gitOutput runs git in dir and returns stdout (trimmed).
func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// flipStoryMerged rewrites the drawer file to set status=merged and merged_at=<now>.
func flipStoryMerged(drawerPath string) error {
	data, err := os.ReadFile(drawerPath)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	updated := rewriteStatusAndMergedAt(string(data), "merged", now)
	if updated == string(data) {
		return fmt.Errorf("drawer %s has malformed frontmatter (no `---` markers found); cannot flip status", drawerPath)
	}
	tmp := drawerPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(updated), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Clean(drawerPath))
}

// rewriteStatusAndMergedAt updates the status: line inside the leading YAML frontmatter
// and adds (or replaces) a merged_at: line. We do this textually instead of round-tripping
// through gopkg.in/yaml.v3 because that library re-orders keys and strips comments — both of
// which would noisily churn the drawer files.
func rewriteStatusAndMergedAt(src, newStatus, mergedAt string) string {
	const marker = "---\n"
	if !strings.HasPrefix(src, marker) {
		return src
	}
	rest := src[len(marker):]
	end := strings.Index(rest, "\n"+marker)
	if end < 0 {
		return src
	}
	frontmatter := rest[:end+1] // include trailing newline
	body := rest[end+len(marker)+1:]

	lines := strings.Split(strings.TrimSuffix(frontmatter, "\n"), "\n")
	wroteStatus := false
	wroteMergedAt := false
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "status:"):
			lines[i] = "status: " + newStatus
			wroteStatus = true
		case strings.HasPrefix(line, "merged_at:"):
			lines[i] = "merged_at: " + mergedAt
			wroteMergedAt = true
		}
	}
	if !wroteStatus {
		lines = append(lines, "status: "+newStatus)
	}
	if !wroteMergedAt {
		lines = append(lines, "merged_at: "+mergedAt)
	}
	newFrontmatter := strings.Join(lines, "\n") + "\n"
	return marker + newFrontmatter + marker + body
}
