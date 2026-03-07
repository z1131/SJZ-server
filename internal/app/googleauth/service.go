package googleauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/auth"
	"github.com/sipeed/picoclaw/pkg/providers"
)

const (
	providerName = "google"
	providerID   = "google-antigravity"
)

// Service manages Google OAuth authentication for the UI.
type Service struct {
	mu          sync.RWMutex
	server      *http.Server
	listener    net.Listener
	session     *loginSession
	subscribers map[chan Status]struct{}
	cfg         auth.OAuthProviderConfig
}

type loginSession struct {
	State       string
	PKCE        auth.PKCECodes
	RedirectURI string
	Status      string
	Error       string
	Done        bool
	StartedAt   time.Time
}

// Status represents the current authentication status.
type Status struct {
	Provider   string `json:"provider"`
	Status     string `json:"status"`
	Connected  bool   `json:"connected"`
	AuthMethod string `json:"auth_method,omitempty"`
	AccountID  string `json:"account_id,omitempty"`
	Email      string `json:"email,omitempty"`
	ProjectID  string `json:"project_id,omitempty"`
	Error      string `json:"error,omitempty"`
}

// StartLoginResult contains the result of starting a login flow.
type StartLoginResult struct {
	Status    string `json:"status"`
	VerifyURL string `json:"verify_url"`
}

// NewService creates a new Google OAuth service.
func NewService() *Service {
	return &Service{
		subscribers: make(map[chan Status]struct{}),
		cfg:         auth.GoogleAntigravityOAuthConfig(),
	}
}

// StartLogin initiates the OAuth login flow.
func (s *Service) StartLogin() (*StartLoginResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if there's an ongoing session
	if s.session != nil && !s.session.Done && s.session.Status == "pending" {
		return &StartLoginResult{
			Status:    "pending",
			VerifyURL: s.buildAuthURL(),
		}, nil
	}

	// Stop any existing server
	s.stopServerLocked()

	pkce, err := auth.GeneratePKCE()
	if err != nil {
		return nil, fmt.Errorf("generating PKCE: %w", err)
	}

	state, err := auth.GenerateState()
	if err != nil {
		return nil, fmt.Errorf("generating state: %w", err)
	}

	redirectURI := fmt.Sprintf("http://localhost:%d/auth/callback", s.cfg.Port)

	// Start callback server
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.cfg.Port))
	if err != nil {
		return nil, fmt.Errorf("starting callback server: %w", err)
	}

	s.listener = listener
	s.session = &loginSession{
		State:       state,
		PKCE:        pkce,
		RedirectURI: redirectURI,
		Status:      "pending",
		StartedAt:   time.Now(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", s.handleCallback)

	s.server = &http.Server{Handler: mux}
	go s.server.Serve(s.listener)

	// Set timeout
	go s.timeoutMonitor()

	return &StartLoginResult{
		Status:    "pending",
		VerifyURL: s.buildAuthURL(),
	}, nil
}

func (s *Service) buildAuthURL() string {
	return auth.BuildAuthorizeURL(s.cfg, s.session.PKCE, s.session.State, s.session.RedirectURI)
}

func (s *Service) handleCallback(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.session == nil || s.session.Done {
		http.Error(w, "No active session", http.StatusBadRequest)
		return
	}

	if r.URL.Query().Get("state") != s.session.State {
		s.session.Status = "error"
		s.session.Error = "State mismatch"
		s.session.Done = true
		http.Error(w, "State mismatch", http.StatusBadRequest)
		go s.broadcast(s.currentStatusLocked())
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		errMsg := r.URL.Query().Get("error")
		s.session.Status = "error"
		s.session.Error = fmt.Sprintf("No authorization code: %s", errMsg)
		s.session.Done = true
		http.Error(w, "No authorization code", http.StatusBadRequest)
		go s.broadcast(s.currentStatusLocked())
		return
	}

	// Success page
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, `<html><body><h2>Google Authentication successful!</h2><p>You can close this window.</p></body></html>`)

	// Exchange code for tokens
	go s.completeLogin(code)
}

func (s *Service) completeLogin(code string) {
	s.mu.RLock()
	session := s.session
	s.mu.RUnlock()

	if session == nil {
		return
	}

	cred, err := auth.ExchangeCodeForTokens(s.cfg, code, session.PKCE.CodeVerifier, session.RedirectURI)
	if err != nil {
		s.finishSession(session, "error", fmt.Sprintf("Token exchange failed: %s", err.Error()))
		return
	}

	cred.Provider = providerID

	// Fetch user email
	email, err := fetchGoogleUserEmail(cred.AccessToken)
	if err != nil {
		s.finishSession(session, "error", fmt.Sprintf("Failed to fetch user info: %s", err.Error()))
		return
	}
	cred.Email = email

	// Fetch project ID
	projectID, err := providers.FetchAntigravityProjectID(cred.AccessToken)
	if err != nil {
		// Non-fatal: use fallback project ID
		projectID = "rising-fact-p41fc"
	}
	cred.ProjectID = projectID

	// Save credentials
	if err := auth.SetCredential(providerID, cred); err != nil {
		s.finishSession(session, "error", fmt.Sprintf("Failed to save credentials: %s", err.Error()))
		return
	}

	s.finishSession(session, "success", "")
}

func (s *Service) timeoutMonitor() {
	time.Sleep(5 * time.Minute)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.session != nil && !s.session.Done && s.session.Status == "pending" {
		s.session.Status = "error"
		s.session.Error = "Login timed out after 5 minutes"
		s.session.Done = true
		s.stopServerLocked()
		go s.broadcast(s.currentStatusLocked())
	}
}

func (s *Service) finishSession(session *loginSession, status, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.session != session {
		return
	}

	s.session.Status = status
	s.session.Error = errMsg
	s.session.Done = true
	// Broadcast status BEFORE stopping the server to ensure clients receive the update
	s.broadcast(s.currentStatusLocked())
	s.stopServerLocked()
}

func (s *Service) stopServerLocked() {
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		s.server.Shutdown(ctx)
		s.server = nil
	}
	if s.listener != nil {
		s.listener.Close()
		s.listener = nil
	}
}

// GetStatus returns the current authentication status.
func (s *Service) GetStatus() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentStatusLocked()
}

// Subscribe returns a channel for receiving status updates.
func (s *Service) Subscribe() chan Status {
	ch := make(chan Status, 8)
	s.mu.Lock()
	s.subscribers[ch] = struct{}{}
	status := s.currentStatusLocked()
	s.mu.Unlock()
	ch <- status
	return ch
}

// Unsubscribe removes a subscriber.
func (s *Service) Unsubscribe(ch chan Status) {
	s.mu.Lock()
	if _, ok := s.subscribers[ch]; ok {
		delete(s.subscribers, ch)
		close(ch)
	}
	s.mu.Unlock()
}

func (s *Service) broadcast(status Status) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for ch := range s.subscribers {
		select {
		case ch <- status:
		default:
		}
	}
}

func (s *Service) currentStatusLocked() Status {
	status := Status{
		Provider: providerName,
		Status:   "idle",
	}

	cred, err := auth.GetCredential(providerID)
	if err == nil && cred != nil {
		status.AuthMethod = cred.AuthMethod
		status.Email = cred.Email
		status.ProjectID = cred.ProjectID
		if !cred.IsExpired() || cred.RefreshToken != "" {
			status.Connected = true
			status.Status = "connected"
		}
	}

	if s.session != nil {
		switch s.session.Status {
		case "pending":
			status.Status = "pending"
		case "error":
			status.Status = "error"
			status.Error = s.session.Error
		case "success":
			status.Status = "connected"
			status.Connected = true
		}
	}

	return status
}

func fetchGoogleUserEmail(accessToken string) (string, error) {
	req, err := http.NewRequest("GET", "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading userinfo response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("userinfo request failed: %s", string(body))
	}

	var userInfo struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return "", err
	}
	return userInfo.Email, nil
}

// FetchModels fetches available models for Google Antigravity.
func FetchModels(accessToken, projectID string) ([]ModelInfo, error) {
	models, err := providers.FetchAntigravityModels(accessToken, projectID)
	if err != nil {
		return nil, err
	}

	result := make([]ModelInfo, len(models))
	for i, m := range models {
		result[i] = ModelInfo{
			ID:          m.ID,
			DisplayName: m.DisplayName,
			IsExhausted: m.IsExhausted,
		}
	}
	return result, nil
}

// ModelInfo represents a Google Antigravity model.
type ModelInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	IsExhausted bool   `json:"is_exhausted"`
}