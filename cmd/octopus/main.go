
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ilibx/octopus/cmd/octopus/internal"
	"github.com/ilibx/octopus/cmd/octopus/internal/agent"
	"github.com/ilibx/octopus/cmd/octopus/internal/auth"
	"github.com/ilibx/octopus/cmd/octopus/internal/cron"
	"github.com/ilibx/octopus/cmd/octopus/internal/gateway"
	"github.com/ilibx/octopus/cmd/octopus/internal/migrate"
	"github.com/ilibx/octopus/cmd/octopus/internal/model"
	"github.com/ilibx/octopus/cmd/octopus/internal/onboard"
	"github.com/ilibx/octopus/cmd/octopus/internal/skills"
	"github.com/ilibx/octopus/cmd/octopus/internal/status"
	"github.com/ilibx/octopus/cmd/octopus/internal/version"
	"github.com/ilibx/octopus/pkg/config"
)

func NewOctopusCommand() *cobra.Command {
	var configPath string

	short := fmt.Sprintf("%s octopus - Personal AI Assistant v%s\n\n", internal.Logo, config.GetVersion())

	cmd := &cobra.Command{
		Use:     "octopus",
		Short:   short,
		Example: "octopus gateway --config /path/to/config.json",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if configPath != "" {
				internal.SetConfigPath(configPath)
			}
			return nil
		},
	}

	cmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "Path to config file (default: $OCTOPUS_CONFIG or ~/.octopus/config.json)")

	cmd.AddCommand(
		onboard.NewOnboardCommand(),
		agent.NewAgentCommand(),
		auth.NewAuthCommand(),
		gateway.NewGatewayCommand(),
		status.NewStatusCommand(),
		cron.NewCronCommand(),
		migrate.NewMigrateCommand(),
		skills.NewSkillsCommand(),
		model.NewModelCommand(),
		version.NewVersionCommand(),
	)

	return cmd
}

const (
	colorBlue = "\033[1;38;2;62;93;185m"
	colorRed  = "\033[1;38;2;213;70;70m"
	banner    = "\r\n" +
		colorBlue + " ██████╗ ██╗   ██╗██████╗ \n" +
		colorBlue + "██╔═══██╗██║   ██║██╔══██╗\n" +
		colorBlue + "██║   ██║██║   ██║██║  ██║\n" +
		colorBlue + "██║▄▄ ██║██║   ██║██║  ██║\n" +
		colorBlue + "╚██████╔╝╚██████╔╝██████╔╝\n" +
		colorBlue + " ╚══▀▀═╝  ╚═════╝ ╚═════╝ \n" +
		colorRed + "██████╗ ██╗ ██████╗ ██████╗  ██████╗██╗      █████╗ ██╗    ██╗\n" +
		colorRed + "██╔══██╗██║██╔════╝██╔═══██╗██╔════╝██║     ██╔══██╗██║    ██║\n" +
		colorRed + "██████╔╝██║██║     ██║   ██║██║     ██║     ███████║██║ █╗ ██║\n" +
		colorRed + "██╔═══╝ ██║██║     ██║   ██║██║     ██║     ██╔══██║██║███╗██║\n" +
		colorRed + "██║     ██║╚██████╗╚██████╔╝╚██████╗███████╗██║  ██║╚███╔███╔╝\n" +
		colorRed + "╚═╝     ╚═╝ ╚═════╝ ╚═════╝  ╚═════╝╚══════╝╚═╝  ╚═╝ ╚══╝╚══╝ \n" +
		"\033[0m\r\n"
)

func main() {
	fmt.Printf("%s", banner)
	cmd := NewOctopusCommand()
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
