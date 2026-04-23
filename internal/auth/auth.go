// Package auth handles Gmail OAuth2 credential loading and token management.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"

	"github.com/kpiwko/gmail-mcp-server/internal/config"
)

// ErrNoToken is returned when no usable cached token exists.
var ErrNoToken = errors.New("no cached token — authorization required")

// credentialsFile matches the JSON downloaded from Google Cloud Console.
type credentialsFile struct {
	Installed struct {
		ClientID     string   `json:"client_id"`
		ClientSecret string   `json:"client_secret"`
		RedirectURIs []string `json:"redirect_uris"`
	} `json:"installed"`
}

// LoadCredentials resolves the OAuth client ID and secret.
// Priority: environment variables > credentials.json in the config directory.
func LoadCredentials() (clientID, clientSecret string, err error) {
	clientID = os.Getenv("GMAIL_CLIENT_ID")
	clientSecret = os.Getenv("GMAIL_CLIENT_SECRET")
	if clientID != "" && clientSecret != "" {
		return clientID, clientSecret, nil
	}

	credPath := config.AppFilePath("credentials.json")
	data, readErr := os.ReadFile(credPath)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return "", "", fmt.Errorf(
				"no credentials found — set GMAIL_CLIENT_ID + GMAIL_CLIENT_SECRET, "+
					"or place credentials.json in %s", config.AppDataDir())
		}
		return "", "", fmt.Errorf("failed to read %s: %w", credPath, readErr)
	}

	var f credentialsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return "", "", fmt.Errorf("failed to parse %s: %w", credPath, err)
	}
	if f.Installed.ClientID == "" || f.Installed.ClientSecret == "" {
		return "", "", fmt.Errorf("%s is missing client_id or client_secret", credPath)
	}
	return f.Installed.ClientID, f.Installed.ClientSecret, nil
}

// OAuthConfig returns an oauth2.Config for Gmail.
func OAuthConfig(clientID, clientSecret string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       []string{gmail.GmailReadonlyScope, gmail.GmailComposeScope},
		Endpoint:     google.Endpoint,
	}
}

// GetCachedToken loads a usable token from the cache file.
// Returns ErrNoToken if no file exists or the token is expired with no refresh token.
func GetCachedToken() (*oauth2.Token, error) {
	token, err := tokenFromFile(config.AppFilePath("token.json"))
	if err != nil {
		return nil, ErrNoToken
	}
	if token.RefreshToken == "" && !token.Valid() {
		return nil, ErrNoToken
	}
	log.Println("✅ Using cached token")
	return token, nil
}

// DeviceFlow tracks an in-progress Google Device Authorization Grant (RFC 8628).
type DeviceFlow struct {
	// VerificationURI is the URL the user should open. May have the code embedded.
	VerificationURI string
	// UserCode is the short code to enter at the verification page.
	UserCode string

	done  chan struct{}
	token *oauth2.Token
	err   error
}

// Wait returns a channel that is closed when the flow completes (success or error).
func (df *DeviceFlow) Wait() <-chan struct{} { return df.done }

// IsComplete reports whether the flow has finished without blocking.
func (df *DeviceFlow) IsComplete() bool {
	select {
	case <-df.done:
		return true
	default:
		return false
	}
}

// Token returns the authorized token. Only valid after Wait() is closed.
func (df *DeviceFlow) Token() *oauth2.Token { return df.token }

// Err returns any error from the flow. Only valid after Wait() is closed.
func (df *DeviceFlow) Err() error { return df.err }

// StartDeviceFlow begins a Google Device Authorization Grant.
// It polls Google in the background; check IsComplete() or receive on Wait().
func StartDeviceFlow(ctx context.Context, cfg *oauth2.Config) (*DeviceFlow, error) {
	da, err := cfg.DeviceAuth(ctx, oauth2.AccessTypeOffline)
	if err != nil {
		return nil, fmt.Errorf("starting device authorization: %w", err)
	}

	uri := da.VerificationURIComplete
	if uri == "" {
		uri = da.VerificationURI
	}

	df := &DeviceFlow{
		VerificationURI: uri,
		UserCode:        da.UserCode,
		done:            make(chan struct{}),
	}

	go func() {
		defer close(df.done)
		token, err := cfg.DeviceAccessToken(ctx, da)
		if err != nil {
			df.err = fmt.Errorf("device authorization: %w", err)
			log.Printf("❌ Gmail device authorization failed: %v", err)
			return
		}
		saveToken(config.AppFilePath("token.json"), token)
		df.token = token
		log.Println("✅ Gmail authorization complete — token saved")
	}()

	return df, nil
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	token := &oauth2.Token{}
	return token, json.NewDecoder(f).Decode(token)
}

func saveToken(path string, token *oauth2.Token) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		log.Printf("Unable to cache OAuth token: %v", err)
		return
	}
	defer func() { _ = f.Close() }()
	_ = json.NewEncoder(f).Encode(token)
}
