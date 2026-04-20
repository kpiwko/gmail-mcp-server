// Package gmail implements the Gmail MCP tool handlers.
package gmail

import (
	"context"
	"fmt"

	"google.golang.org/api/gmail/v1"
	googleOption "google.golang.org/api/option"

	"github.com/kpiwko/gmail-mcp-server/internal/auth"
)

// Server wraps the Gmail API service and exposes MCP tool methods.
type Server struct {
	service *gmail.Service
	userID  string
}

// NewServer authenticates with Gmail and returns a ready-to-use Server.
func NewServer() (*Server, error) {
	clientID, clientSecret, err := auth.LoadCredentials()
	if err != nil {
		return nil, err
	}

	cfg := auth.OAuthConfig(clientID, clientSecret)
	token, err := auth.GetToken(cfg)
	if err != nil {
		return nil, fmt.Errorf("unable to get token: %v", err)
	}

	ctx := context.Background()
	client := cfg.Client(ctx, token)
	svc, err := gmail.NewService(ctx, googleOption.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to create Gmail service: %v", err)
	}

	return &Server{service: svc, userID: "me"}, nil
}

// Profile returns the authenticated user's Gmail profile.
// Used at startup in HTTP mode to verify authentication before accepting clients.
func (s *Server) Profile() (*gmail.Profile, error) {
	profile, err := s.service.Users.GetProfile(s.userID).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get user profile: %v", err)
	}
	return profile, nil
}

