package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nikrich/hungry-ghost-hive-v2/internal/drawers"
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

// runGitMerge is implemented in Task 4. Stubbed here so the file compiles.
func runGitMerge(workspaceRoot, team, featureBranch, agentBranch string) error {
	return errors.New("runGitMerge not yet implemented")
}

// flipStoryMerged rewrites the drawer file to set status=merged and merged_at=<now>.
func flipStoryMerged(drawerPath string) error {
	data, err := os.ReadFile(drawerPath)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	updated := rewriteStatusAndMergedAt(string(data), "merged", now)
	tmp := drawerPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(updated), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Clean(drawerPath))
}

// rewriteStatusAndMergedAt does a minimal frontmatter rewrite without a full YAML round-trip
// so we preserve any comments and field order present in the source file.
func rewriteStatusAndMergedAt(src, newStatus, mergedAt string) string {
	// Implemented in Task 5. Skeleton returns src so the refusal test compiles.
	_ = newStatus
	_ = mergedAt
	return src
}
