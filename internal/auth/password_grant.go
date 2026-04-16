package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TokenResponse represents the response from a Keycloak token endpoint.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

func tokenEndpoint(issuerURL string) string {
	return strings.TrimRight(issuerURL, "/") + "/protocol/openid-connect/token"
}

// PasswordGrant performs a Resource Owner Password Credentials grant against Keycloak.
func PasswordGrant(issuerURL, clientID, clientSecret, username, password string) (*TokenResponse, error) {
	data := url.Values{
		"grant_type": {"password"},
		"client_id":  {clientID},
		"username":   {username},
		"password":   {password},
	}
	if clientSecret != "" {
		data.Set("client_secret", clientSecret)
	}

	resp, err := httpClient.PostForm(tokenEndpoint(issuerURL), data)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Keycloak: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("authentication failed (%d): %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	return &tokenResp, nil
}

// RefreshToken exchanges a refresh token for a new access token.
func RefreshToken(issuerURL, clientID, clientSecret, refreshToken string) (*TokenResponse, error) {
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {clientID},
		"refresh_token": {refreshToken},
	}
	if clientSecret != "" {
		data.Set("client_secret", clientSecret)
	}

	resp, err := httpClient.PostForm(tokenEndpoint(issuerURL), data)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Keycloak: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token refresh failed (%d): %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	return &tokenResp, nil
}
