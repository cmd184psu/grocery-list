package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const DefaultConfigPath = "~/.grocery.json"

type Config struct {
	Port      int      `json:"port"`
	TLSCert   string   `json:"tls_cert"`
	TLSKey    string   `json:"tls_key"`
	StaticDir string   `json:"static_dir"`
	DataFile  string   `json:"data_file"`
	Groups    []string `json:"groups"`
}

func DefaultConfig() *Config {
	return &Config{
		Port:      8080,
		StaticDir: "./web",
		DataFile:  "./items.json",
		Groups: []string{
			"Produce",
			"Meats",
			"mid store",
			"back wall",
			"frozen",
			"deli area near front",
		},
	}
}

// ExpandPath expands a leading ~ to the user home directory.
func ExpandPath(path string) (string, error) {
	if len(path) == 0 || path[0] != '~' {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	return filepath.Join(home, path[1:]), nil
}

// Load reads the config file at path (supports ~). Missing file returns defaults.
func Load(path string) (*Config, error) {
	expanded, err := ExpandPath(path)
	if err != nil {
		return nil, err
	}

	cfg := DefaultConfig()

	data, err := os.ReadFile(expanded)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", expanded, err)
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", expanded, err)
	}

	// Expand ~ in paths inside the config file
	if cfg.TLSCert != "" {
		if cfg.TLSCert, err = ExpandPath(cfg.TLSCert); err != nil {
			return nil, err
		}
	}
	if cfg.TLSKey != "" {
		if cfg.TLSKey, err = ExpandPath(cfg.TLSKey); err != nil {
			return nil, err
		}
	}
	if cfg.StaticDir, err = ExpandPath(cfg.StaticDir); err != nil {
		return nil, err
	}
	if cfg.DataFile, err = ExpandPath(cfg.DataFile); err != nil {
		return nil, err
	}

	return cfg, nil
}

// WriteDefault writes a default config to path if it doesn't already exist.
func WriteDefault(path string) error {
	expanded, err := ExpandPath(path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(expanded); err == nil {
		return nil // already exists
	}
	data, err := json.MarshalIndent(DefaultConfig(), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(expanded, data, 0644)
}
