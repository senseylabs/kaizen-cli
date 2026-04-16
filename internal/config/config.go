package config

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds the configuration loaded from kaizen.yaml files and environment variables.
type Config struct {
	APIURL       string `mapstructure:"api-url"`
	Issuer       string `mapstructure:"issuer"`
	OrgID        string `mapstructure:"org-id"`
	DefaultBoard string `mapstructure:"default-board"`
}

// Load reads configuration from kaizen.yaml in the current directory,
// then falls back to ~/.kaizen/config.yaml. CWD values take priority.
// Environment variables (KAIZEN_*) override both.
func Load() Config {
	var cfg Config

	// Second priority: ~/.kaizen/config.yaml (load first, will be overridden by CWD)
	home, err := os.UserHomeDir()
	if err == nil {
		hv := viper.New()
		hv.SetConfigName("config")
		hv.SetConfigType("yaml")
		hv.AddConfigPath(filepath.Join(home, ".kaizen"))
		if err := hv.ReadInConfig(); err == nil {
			_ = hv.Unmarshal(&cfg)
		}
	}

	// First priority: kaizen.yaml in current working directory (overrides home config)
	cwd, err := os.Getwd()
	if err == nil {
		cv := viper.New()
		cv.SetConfigName("kaizen")
		cv.SetConfigType("yaml")
		cv.AddConfigPath(cwd)
		if err := cv.ReadInConfig(); err == nil {
			var cwdCfg Config
			if err := cv.Unmarshal(&cwdCfg); err == nil {
				if cwdCfg.APIURL != "" {
					cfg.APIURL = cwdCfg.APIURL
				}
				if cwdCfg.Issuer != "" {
					cfg.Issuer = cwdCfg.Issuer
				}
				if cwdCfg.OrgID != "" {
					cfg.OrgID = cwdCfg.OrgID
				}
				if cwdCfg.DefaultBoard != "" {
					cfg.DefaultBoard = cwdCfg.DefaultBoard
				}
			}
		}
	}

	// Environment variables override everything
	if v := os.Getenv("KAIZEN_API_URL"); v != "" {
		cfg.APIURL = v
	}
	if v := os.Getenv("KAIZEN_KEYCLOAK_ISSUER"); v != "" {
		cfg.Issuer = v
	}
	if v := os.Getenv("KAIZEN_ORG_ID"); v != "" {
		cfg.OrgID = v
	}
	if v := os.Getenv("KAIZEN_DEFAULT_BOARD"); v != "" {
		cfg.DefaultBoard = v
	}

	return cfg
}
