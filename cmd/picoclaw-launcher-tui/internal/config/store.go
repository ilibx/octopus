package configstore

import (
	"errors"
	"os"
	"path/filepath"

	octopusconfig "github.com/ilibx/octopus/pkg/config"
)

const (
	configDirName  = ".octopus"
	configFileName = "config.json"
)

func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configDirName), nil
}

func Load() (*octopusconfig.Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	return octopusconfig.LoadConfig(path)
}

func Save(cfg *octopusconfig.Config) error {
	if cfg == nil {
		return errors.New("config is nil")
	}
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	return octopusconfig.SaveConfig(path, cfg)
}
