// Package gmail implements the Gmail MCP tool handlers.
package gmail

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"golang.org/x/oauth2"
	"google.golang.org/api/gmail/v1"
	googleOption "google.golang.org/api/option"
)

// ErrNotAuthenticated is returned by tool methods when no token has been set yet.
var ErrNotAuthenticated = errors.New("not authenticated — visit /auth to authorize Gmail access")

// Server wraps the Gmail API service and exposes MCP tool methods.
// It is safe to use before authentication; calls return ErrNotAuthenticated
// until Authenticate is called.
type Server struct {
	mu      sync.RWMutex
	service *gmail.Service
	userID  string
}

// New returns a ready-to-use Server authenticated with the given token.
func New(token *oauth2.Token, cfg *oauth2.Config) (*Server, error) {
	s := &Server{userID: "me"}
	if err := s.Authenticate(token, cfg); err != nil {
		return nil, err
	}
	return s, nil
}

// NewPending returns an unauthenticated Server.
// All tool-method calls will return ErrNotAuthenticated until Authenticate is called.
func NewPending() *Server {
	return &Server{userID: "me"}
}

// Authenticate initializes the Gmail API client from the given token and config.
// Safe to call concurrently; subsequent calls replace the previous client.
func (s *Server) Authenticate(token *oauth2.Token, cfg *oauth2.Config) error {
	client := cfg.Client(context.Background(), token)
	svc, err := gmail.NewService(context.Background(), googleOption.WithHTTPClient(client))
	if err != nil {
		return fmt.Errorf("unable to create Gmail service: %w", err)
	}
	s.mu.Lock()
	s.service = svc
	s.mu.Unlock()
	return nil
}

// IsAuthenticated reports whether the server has a valid Gmail API client.
func (s *Server) IsAuthenticated() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.service != nil
}

// svc returns the Gmail service or ErrNotAuthenticated if not yet authorized.
func (s *Server) svc() (*gmail.Service, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.service == nil {
		return nil, ErrNotAuthenticated
	}
	return s.service, nil
}

// Profile returns the authenticated user's Gmail profile.
// Used at startup in HTTP mode to verify authentication.
func (s *Server) Profile() (*gmail.Profile, error) {
	svc, err := s.svc()
	if err != nil {
		return nil, err
	}
	profile, err := svc.Users.GetProfile(s.userID).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get user profile: %w", err)
	}
	return profile, nil
}
