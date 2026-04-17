package internal

import (
	"os"
	"path/filepath"

	"github.com/ilibx/octopus/pkg/config"
)

const Logo = "🦞"

// GetOctopusHome returns the octopus home directory.
// Priority: $OCTOPUS_HOME > ~/.octopus
func GetOctopusHome() string {
	if home := os.Getenv("OCTOPUS_HOME"); home != "" {
		return home
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".octopus")
}

func GetConfigPath() string {
	if configPath := os.Getenv("OCTOPUS_CONFIG"); configPath != "" {
		return configPath
	}
	return filepath.Join(GetOctopusHome(), "config.json")
}

func LoadConfig() (*config.Config, error) {
	return config.LoadConfig(GetConfigPath())
}

// FormatVersion returns the version string with optional git commit
// Deprecated: Use pkg/config.FormatVersion instead
func FormatVersion() string {
	return config.FormatVersion()
}

// FormatBuildInfo returns build time and go version info
// Deprecated: Use pkg/config.FormatBuildInfo instead
func FormatBuildInfo() (string, string) {
	return config.FormatBuildInfo()
}

// GetVersion returns the version string
// Deprecated: Use pkg/config.GetVersion instead
func GetVersion() string {
	return config.GetVersion()
}
