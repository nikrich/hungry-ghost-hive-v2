package cli

import (
	"fmt"
	"io"

	"github.com/nikrich/hungry-ghost-hive-v2/internal/drawers"
)

// RenderStatus writes a status summary for the given wing root to out.
func RenderStatus(out io.Writer, wingRoot string) error {
	stories, err := drawers.List(wingRoot, "stories")
	if err != nil {
		return err
	}
	agents, err := drawers.List(wingRoot, "agents")
	if err != nil {
		return err
	}
	escalations, err := drawers.List(wingRoot, "escalations")
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "stories: %d\n", len(stories))
	statusCounts := map[string]int{}
	for _, s := range stories {
		statusCounts[s.Status]++
	}
	for _, k := range []string{"pending", "assigned", "in-progress", "review", "merged", "blocked", "abandoned"} {
		if c := statusCounts[k]; c > 0 {
			fmt.Fprintf(out, "  %s: %d\n", k, c)
		}
	}

	live := drawers.FilterByStatus(agents, "live")
	fmt.Fprintf(out, "agents live: %d\n", len(live))
	for _, a := range live {
		fmt.Fprintf(out, "  %s (%s) -> %s\n", a.Title, a.Role, a.Story)
	}

	open := drawers.FilterByStatus(escalations, "open")
	fmt.Fprintf(out, "open escalations: %d\n", len(open))
	for _, e := range open {
		fmt.Fprintf(out, "  %s\n", e.Title)
	}
	return nil
}
