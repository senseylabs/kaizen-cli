package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/senseylabs/kaizen-cli/internal/auth"
	"github.com/senseylabs/kaizen-cli/internal/client"
	"github.com/senseylabs/kaizen-cli/internal/config"
	"github.com/senseylabs/kaizen-cli/internal/update"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const (
	prodAPIURL   = "https://api.sensey.io"
	prodIssuer   = "https://keycloak.sensey.io/realms/sensey"
	prodClientID = "kaizen-cli"
	devAPIURL    = "http://localhost:8080"
	devIssuer    = "http://localhost:8086/realms/sensey"
	devClientID  = "village-jwt-test-client"
)

var (
	cfgAPIURL       string
	cfgIssuer       string
	cfgClientID     string
	cfgOrgID        string
	cfgDefaultBoard string
	cfgDevMode      bool
	cfgDebug        bool
	cfgJSON         bool
	appVersion      string
)

// SetVersion sets the application version (called from main via ldflags).
func SetVersion(v string) {
	appVersion = v
	rootCmd.Version = v
}

var rootCmd = &cobra.Command{
	Use:   "kaizen",
	Short: "Kaizen CLI — project management from your terminal",
	Long:  "A CLI tool for managing boards, tickets, sprints, and backlogs in Kaizen. Supports Keycloak Device Authorization Grant for interactive login and Personal Access Tokens for CI/CD.",
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if cfgJSON {
			return
		}
		latestVersion := update.CheckForUpdate(appVersion)
		if latestVersion == "" {
			return
		}
		useColor := term.IsTerminal(int(os.Stderr.Fd()))
		_, _ = fmt.Fprint(os.Stderr, update.FormatNotice(appVersion, latestVersion, useColor))
	},
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		var notFound *client.NotFoundError
		var forbidden *client.ForbiddenError
		if errors.As(err, &notFound) {
			os.Exit(2)
		} else if errors.As(err, &forbidden) {
			os.Exit(3)
		}
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgAPIURL, "api-url", "", "Kaizen API base URL")
	rootCmd.PersistentFlags().StringVar(&cfgIssuer, "issuer", "", "Keycloak issuer URL")
	rootCmd.PersistentFlags().StringVar(&cfgClientID, "client-id", "", "Keycloak client ID")
	rootCmd.PersistentFlags().StringVar(&cfgOrgID, "org", "", "Organization ID")
	rootCmd.PersistentFlags().StringVar(&cfgDefaultBoard, "board", "", "Default board name or ID")
	rootCmd.PersistentFlags().BoolVar(&cfgDevMode, "dev", false, "Use local development URLs (localhost)")
	rootCmd.PersistentFlags().BoolVar(&cfgDebug, "debug", false, "Enable debug logging for HTTP requests")
	rootCmd.PersistentFlags().BoolVar(&cfgJSON, "json", false, "Output raw JSON instead of human-readable format")

	rootCmd.Version = appVersion
}

func initConfig() {
	cfg := config.Load()

	// Load stored credentials for fallback values
	store := auth.NewCredentialStore()
	storedCreds, err := store.Load()
	if err != nil {
		if !strings.Contains(err.Error(), "no credentials found") {
			_, _ = fmt.Fprintf(os.Stderr, "Warning: could not load stored credentials: %v\n", err)
		}
	}

	// If --dev not explicitly set, check stored credentials
	if !cfgDevMode && storedCreds.DevMode {
		cfgDevMode = true
	}

	// Dev mode defaults
	if cfgDevMode {
		if cfgAPIURL == "" {
			cfgAPIURL = devAPIURL
		}
		if cfgIssuer == "" {
			cfgIssuer = devIssuer
		}
		if cfgClientID == "" {
			cfgClientID = devClientID
		}
	}

	// Resolve API URL: flag → env var → config file → stored creds → production default
	if cfgAPIURL == "" {
		if cfg.APIURL != "" {
			cfgAPIURL = cfg.APIURL
		} else if storedCreds.APIURL != "" {
			cfgAPIURL = storedCreds.APIURL
		} else {
			cfgAPIURL = prodAPIURL
		}
	}

	if cfgIssuer == "" {
		if cfg.Issuer != "" {
			cfgIssuer = cfg.Issuer
		} else if storedCreds.IssuerURL != "" {
			cfgIssuer = storedCreds.IssuerURL
		} else {
			cfgIssuer = prodIssuer
		}
	}

	if cfgClientID == "" {
		if cfg.ClientID != "" {
			cfgClientID = cfg.ClientID
		} else {
			cfgClientID = prodClientID
		}
	}

	if cfgOrgID == "" {
		if cfg.OrgID != "" {
			cfgOrgID = cfg.OrgID
		} else if storedCreds.OrgID != "" {
			cfgOrgID = storedCreds.OrgID
		}
	}

	if cfgDefaultBoard == "" {
		if cfg.DefaultBoard != "" {
			cfgDefaultBoard = cfg.DefaultBoard
		}
	}
}

// requireAuth checks that the user is authenticated.
func requireAuth() error {
	if os.Getenv("KAIZEN_TOKEN") != "" {
		return nil
	}
	store := auth.NewCredentialStore()
	if _, err := store.Load(); err != nil {
		return fmt.Errorf("you are not logged in. Run 'kaizen login' to authenticate")
	}
	return nil
}

// resolveToken returns a valid access token from env var or credential store.
func resolveToken() (string, error) {
	if pat := os.Getenv("KAIZEN_TOKEN"); pat != "" {
		return pat, nil
	}

	store := auth.NewCredentialStore()
	creds, err := store.Load()
	if err != nil {
		return "", fmt.Errorf("not authenticated. Run 'kaizen login' to authenticate")
	}

	return creds.AccessToken, nil
}
