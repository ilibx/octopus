package migrate

import (
	"github.com/spf13/cobra"

	"github.com/ilibx/octopus/pkg/migrate"
)

func NewMigrateCommand() *cobra.Command {
	var opts migrate.Options

	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate from xxxclaw(octopus, etc.) to octopus",
		Args:  cobra.NoArgs,
		Example: `  octopus migrate
  octopus migrate --from octopus
  octopus migrate --dry-run
  octopus migrate --refresh
  octopus migrate --force`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m := migrate.NewMigrateInstance(opts)
			result, err := m.Run(opts)
			if err != nil {
				return err
			}
			if !opts.DryRun {
				m.PrintSummary(result)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false,
		"Show what would be migrated without making changes")
	cmd.Flags().StringVar(&opts.Source, "from", "octopus",
		"Source to migrate from (e.g., octopus)")
	cmd.Flags().BoolVar(&opts.Refresh, "refresh", false,
		"Re-sync workspace files from Octopus (repeatable)")
	cmd.Flags().BoolVar(&opts.ConfigOnly, "config-only", false,
		"Only migrate config, skip workspace files")
	cmd.Flags().BoolVar(&opts.WorkspaceOnly, "workspace-only", false,
		"Only migrate workspace files, skip config")
	cmd.Flags().BoolVar(&opts.Force, "force", false,
		"Skip confirmation prompts")
	cmd.Flags().StringVar(&opts.SourceHome, "source-home", "",
		"Override source home directory (default: ~/.octopus)")
	cmd.Flags().StringVar(&opts.TargetHome, "target-home", "",
		"Override target home directory (default: ~/.octopus)")

	return cmd
}
