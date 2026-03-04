package cmd

import (
	"fmt"
	"os"

	"github.com/maxcelant/git-synced/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func NewConfigCmd(configPath *string) *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage gsync configuration",
	}

	configCmd.AddCommand(newConfigShowCmd(configPath))
	configCmd.AddCommand(newConfigSetCmd(configPath))
	configCmd.AddCommand(newConfigAddCmd(configPath))
	configCmd.AddCommand(newConfigRemoveCmd(configPath))

	return configCmd
}

func newConfigShowCmd(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			return yaml.NewEncoder(os.Stdout).Encode(cfg)
		},
	}
}

func newConfigSetCmd(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "set <field> <value>",
		Short: "Set a top-level config field (format, output_dir)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			field, value := args[0], args[1]
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			switch field {
			case "format":
				cfg.Format = value
			case "out":
				cfg.OutputDir = value
			default:
				return fmt.Errorf("unknown field %q: supported fields are format, out", field)
			}
			if err := config.Save(*configPath, cfg); err != nil {
				return err
			}
			fmt.Printf("set %s = %s\n", field, value)
			return nil
		},
	}
}

func newConfigAddCmd(configPath *string) *cobra.Command {
	var provider string

	addCmd := &cobra.Command{
		Use:   "add",
		Short: "Add an author or repo to a provider",
	}
	addCmd.PersistentFlags().StringVar(&provider, "provider", "", "provider name (defaults to first provider)")

	addCmd.AddCommand(&cobra.Command{
		Use:   "author <name>",
		Short: "Add an author to a provider",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			p, err := findProvider(cfg, provider)
			if err != nil {
				return err
			}
			for _, a := range p.Authors {
				if a == name {
					return fmt.Errorf("author %q already exists in provider %q", name, p.Name)
				}
			}
			p.Authors = append(p.Authors, name)
			setProvider(&cfg, p)
			if err := config.Save(*configPath, cfg); err != nil {
				return err
			}
			fmt.Printf("added author %q to provider %q\n", name, p.Name)
			return nil
		},
	})

	addCmd.AddCommand(&cobra.Command{
		Use:   "repo <repo>",
		Short: "Add a repo to a provider",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo := args[0]
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			p, err := findProvider(cfg, provider)
			if err != nil {
				return err
			}
			for _, r := range p.Repos {
				if r == repo {
					return fmt.Errorf("repo %q already exists in provider %q", repo, p.Name)
				}
			}
			p.Repos = append(p.Repos, repo)
			setProvider(&cfg, p)
			if err := config.Save(*configPath, cfg); err != nil {
				return err
			}
			fmt.Printf("added repo %q to provider %q\n", repo, p.Name)
			return nil
		},
	})

	return addCmd
}

func newConfigRemoveCmd(configPath *string) *cobra.Command {
	var provider string

	removeCmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove an author or repo from a provider",
	}
	removeCmd.PersistentFlags().StringVar(&provider, "provider", "", "provider name (defaults to first provider)")

	removeCmd.AddCommand(&cobra.Command{
		Use:   "author <name>",
		Short: "Remove an author from a provider",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			p, err := findProvider(cfg, provider)
			if err != nil {
				return err
			}
			updated := p.Authors[:0]
			for _, a := range p.Authors {
				if a != name {
					updated = append(updated, a)
				}
			}
			if len(updated) == len(p.Authors) {
				return fmt.Errorf("author %q not found in provider %q", name, p.Name)
			}
			p.Authors = updated
			setProvider(&cfg, p)
			if err := config.Save(*configPath, cfg); err != nil {
				return err
			}
			fmt.Printf("removed author %q from provider %q\n", name, p.Name)
			return nil
		},
	})

	removeCmd.AddCommand(&cobra.Command{
		Use:   "repo <repo>",
		Short: "Remove a repo from a provider",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo := args[0]
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			p, err := findProvider(cfg, provider)
			if err != nil {
				return err
			}
			updated := p.Repos[:0]
			for _, r := range p.Repos {
				if r != repo {
					updated = append(updated, r)
				}
			}
			if len(updated) == len(p.Repos) {
				return fmt.Errorf("repo %q not found in provider %q", repo, p.Name)
			}
			p.Repos = updated
			setProvider(&cfg, p)
			if err := config.Save(*configPath, cfg); err != nil {
				return err
			}
			fmt.Printf("removed repo %q from provider %q\n", repo, p.Name)
			return nil
		},
	})

	return removeCmd
}

// findProvider returns a copy of the provider matching the given name, or the
// first provider if name is empty.
func findProvider(cfg config.Config, name string) (config.ProviderConfig, error) {
	if len(cfg.Providers) == 0 {
		return config.ProviderConfig{}, fmt.Errorf("no providers in config")
	}
	if name == "" {
		return cfg.Providers[0], nil
	}
	for _, p := range cfg.Providers {
		if p.Name == name {
			return p, nil
		}
	}
	return config.ProviderConfig{}, fmt.Errorf("provider %q not found", name)
}

// setProvider updates the provider in the config by name.
func setProvider(cfg *config.Config, p config.ProviderConfig) {
	for i := range cfg.Providers {
		if cfg.Providers[i].Name == p.Name {
			cfg.Providers[i] = p
			return
		}
	}
}
