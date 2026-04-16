package auth

import "time"

// Credentials holds the authentication tokens and metadata.
type Credentials struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	IssuerURL    string    `json:"issuer_url"`
	APIURL       string    `json:"api_url,omitempty"`
	OrgID        string    `json:"org_id,omitempty"`
	UserID       string    `json:"user_id,omitempty"`
	DevMode      bool      `json:"dev_mode,omitempty"`
}

// CredentialStore defines the interface for credential storage.
type CredentialStore interface {
	Save(creds Credentials) error
	Load() (Credentials, error)
	Delete() error
}
