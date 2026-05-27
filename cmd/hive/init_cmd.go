package main

import (
	"strings"

	"github.com/nikrich/hungry-ghost-hive-v2/internal/cli"
	"github.com/spf13/cobra"
)

func init() {
	var (
		dir, slug string
		teams     []string
		noClone   bool
	)
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new hive workspace",
		RunE: func(c *cobra.Command, args []string) error {
			opts := cli.InitOptions{
				Dir:           dir,
				WorkspaceSlug: slug,
				NoClone:       noClone,
			}
			for _, t := range teams {
				name, url, ok := splitTeam(t)
				if !ok {
					return c.Help()
				}
				opts.Teams = append(opts.Teams, cli.TeamFlag{Name: name, URL: url})
			}
			return cli.RunInit(opts)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".", "workspace directory")
	cmd.Flags().StringVar(&slug, "workspace-slug", "", "workspace slug (used as hive-<slug> wing name)")
	cmd.Flags().StringArrayVar(&teams, "team", nil, "team in name=<n>,url=<u> form (repeatable)")
	cmd.Flags().BoolVar(&noClone, "no-clone", false, "skip cloning team repos")
	rootCmd.AddCommand(cmd)
}

func splitTeam(s string) (name, url string, ok bool) {
	parts := strings.Split(s, ",")
	for _, p := range parts {
		if kv := strings.SplitN(p, "=", 2); len(kv) == 2 {
			switch kv[0] {
			case "name":
				name = kv[1]
			case "url":
				url = kv[1]
			}
		}
	}
	return name, url, name != "" && url != ""
}
