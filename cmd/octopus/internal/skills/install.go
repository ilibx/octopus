package skills

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ilibx/octopus/cmd/octopus/internal"
	"github.com/ilibx/octopus/pkg/skills"
)

func newInstallCommand(installerFn func() (*skills.SkillInstaller, error)) *cobra.Command {
	var registry string

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install skill from GitHub",
		Example: `
octopus skills install ilibx/octopus-skills/weather
octopus skills install --registry clawhub github
`,
		Args: func(cmd *cobra.Command, args []string) error {
			if registry != "" {
				if len(args) != 1 {
					return fmt.Errorf("when --registry is set, exactly 1 argument is required: <slug>")
				}
				return nil
			}

			if len(args) != 1 {
				return fmt.Errorf("exactly 1 argument is required: <github>")
			}

			return nil
		},
		RunE: func(_ *cobra.Command, args []string) error {
			installer, err := installerFn()
			if err != nil {
				return err
			}

			if registry != "" {
				cfg, err := internal.LoadConfig()
				if err != nil {
					return err
				}

				return skillsInstallFromRegistry(cfg, registry, args[0])
			}

			return skillsInstallCmd(installer, args[0])
		},
	}

	cmd.Flags().StringVar(&registry, "registry", "", "Install from registry: --registry <name> <slug>")

	return cmd
}
