// Package auth handles Gmail OAuth2 credential loading and token management.
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"

	"github.com/kpiwko/gmail-mcp-server/internal/config"
)

// credentialsFile matches the JSON downloaded from Google Cloud Console
// (APIs & Services → Credentials → Download OAuth client).
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
		if !os.IsNotExist(readErr) {
			return "", "", fmt.Errorf("failed to read %s: %v", credPath, readErr)
		}
		// File does not exist — fall through to the error below.
	} else {
		var f credentialsFile
		if err := json.Unmarshal(data, &f); err != nil {
			return "", "", fmt.Errorf("failed to parse %s: %v", credPath, err)
		}
		if f.Installed.ClientID == "" || f.Installed.ClientSecret == "" {
			return "", "", fmt.Errorf("%s is missing client_id or client_secret", credPath)
		}
		if clientID == "" {
			clientID = f.Installed.ClientID
		}
		if clientSecret == "" {
			clientSecret = f.Installed.ClientSecret
		}
		return clientID, clientSecret, nil
	}

	return "", "", fmt.Errorf(
		"no credentials found — set GMAIL_CLIENT_ID + GMAIL_CLIENT_SECRET, "+
			"or place credentials.json in %s", config.AppDataDir(),
	)
}

// OAuthConfig returns an oauth2.Config for Gmail.
// RedirectURL is intentionally left empty; GetToken sets it dynamically.
func OAuthConfig(clientID, clientSecret string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       []string{gmail.GmailReadonlyScope, gmail.GmailComposeScope},
		Endpoint:     google.Endpoint,
	}
}

// GetToken retrieves a cached token or starts the OAuth browser flow.
func GetToken(cfg *oauth2.Config) (*oauth2.Token, error) {
	tokenFile := config.AppFilePath("token.json")

	token, err := tokenFromFile(tokenFile)
	if err != nil {
		log.Printf("No token found (%v), starting OAuth flow...", err)
		return performOAuthFlow(cfg, tokenFile)
	}

	// If the access token is expired but we have a refresh token, the oauth2
	// client will refresh it automatically — no need to re-run the full flow.
	if token.RefreshToken == "" && !token.Valid() {
		log.Println("Token expired and no refresh token — starting OAuth flow...")
		return performOAuthFlow(cfg, tokenFile)
	}

	log.Println("✅ Using cached token")
	return token, nil
}

func performOAuthFlow(cfg *oauth2.Config, tokenFile string) (*oauth2.Token, error) {
	token, err := getTokenFromWeb(cfg)
	if err != nil {
		return nil, err
	}
	saveToken(tokenFile, token)
	return token, nil
}

func getTokenFromWeb(cfg *oauth2.Config) (*oauth2.Token, error) {
	codeChan := make(chan string)
	errChan := make(chan error, 1)

	// Bind on :0 so the OS assigns a free port — avoids conflicts with the
	// SSE server or any other process running on the same machine.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to start OAuth callback listener: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	cfg.RedirectURL = fmt.Sprintf("http://localhost:%d", port)

	mux := http.NewServeMux()
	callbackServer := &http.Server{Handler: mux}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errChan <- fmt.Errorf("no code in OAuth callback")
			return
		}
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><head><title>Gmail MCP Server</title></head><body>
<h1>Authorization successful!</h1>
<p>You can close this window and return to your terminal.</p>
</body></html>`)
		codeChan <- code
	})

	go func() {
		if err := callbackServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("OAuth callback server error: %v", err)
		}
	}()

	authURL := cfg.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Println("Opening browser for Gmail authorization...")
	fmt.Printf("If the browser does not open automatically, visit:\n  %s\n", authURL)
	openBrowser(authURL)

	var authCode string
	select {
	case authCode = <-codeChan:
	case err := <-errChan:
		return nil, fmt.Errorf("authorization failed: %v", err)
	case <-time.After(5 * time.Minute):
		return nil, fmt.Errorf("authorization timed out after 5 minutes")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = callbackServer.Shutdown(ctx)

	token, err := cfg.Exchange(context.Background(), authCode)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve token: %v", err)
	}
	fmt.Println("✅ Authorization successful! Token saved.")
	return token, nil
}

func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		fmt.Printf("Could not open browser automatically: %v\n", err)
	}
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	token := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(token)
	return token, err
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
