package cmd

import (
	"github.com/maxcelant/git-synced/internal/config"
	"github.com/maxcelant/git-synced/internal/tui"
	"github.com/spf13/cobra"
)

func NewTuiCmd(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "menu",
		Short: "Interactive menu for browsing PRs",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			return tui.Run(cfg)
		},
	}
}
