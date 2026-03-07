package openaiauth

import (
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/auth"
)

type Service struct {
	mu          sync.RWMutex
	session     *loginSession
	subscribers map[chan Status]struct{}
}

type loginSession struct {
	Info      *auth.DeviceCodeInfo
	Status    string
	Error     string
	Done      bool
	StartedAt time.Time
}

type PendingDevice struct {
	UserCode          string `json:"user_code"`
	VerifyURL         string `json:"verify_url"`
	VerifyURLComplete string `json:"verify_url_complete"`
	ExpiresIn         int    `json:"expires_in"`
}

type Status struct {
	Provider      string         `json:"provider"`
	Status        string         `json:"status"`
	Connected     bool           `json:"connected"`
	AuthMethod    string         `json:"auth_method,omitempty"`
	AccountID     string         `json:"account_id,omitempty"`
	Email         string         `json:"email,omitempty"`
	Error         string         `json:"error,omitempty"`
	PendingDevice *PendingDevice `json:"pending_device,omitempty"`
}

type StartLoginResult struct {
	Status            string `json:"status"`
	UserCode          string `json:"user_code"`
	VerifyURL         string `json:"verify_url"`
	VerifyURLComplete string `json:"verify_url_complete"`
	ExpiresIn         int    `json:"expires_in"`
}

func NewService() *Service {
	return &Service{
		subscribers: make(map[chan Status]struct{}),
	}
}

func (s *Service) StartLogin() (*StartLoginResult, error) {
	s.mu.RLock()
	if s.session != nil && !s.session.Done && s.session.Status == "pending" {
		info := s.session.Info
		s.mu.RUnlock()
		return &StartLoginResult{
			Status:            "pending",
			UserCode:          info.UserCode,
			VerifyURL:         info.VerifyURL,
			VerifyURLComplete: info.VerifyURL,
			ExpiresIn:         900,
		}, nil
	}
	s.mu.RUnlock()

	oauthCfg := auth.OpenAIOAuthConfig()
	info, err := auth.RequestDeviceCode(oauthCfg)
	if err != nil {
		return nil, err
	}

	session := &loginSession{
		Info:      info,
		Status:    "pending",
		StartedAt: time.Now(),
	}

	s.mu.Lock()
	s.session = session
	s.mu.Unlock()

	s.broadcast(s.currentStatusLocked())
	go s.poll(session)

	return &StartLoginResult{
		Status:            "pending",
		UserCode:          info.UserCode,
		VerifyURL:         info.VerifyURL,
		VerifyURLComplete: info.VerifyURL,
		ExpiresIn:         900,
	}, nil
}

func (s *Service) GetStatus() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentStatusLocked()
}

func (s *Service) Subscribe() chan Status {
	ch := make(chan Status, 8)
	s.mu.Lock()
	s.subscribers[ch] = struct{}{}
	status := s.currentStatusLocked()
	s.mu.Unlock()
	ch <- status
	return ch
}

func (s *Service) Unsubscribe(ch chan Status) {
	s.mu.Lock()
	if _, ok := s.subscribers[ch]; ok {
		delete(s.subscribers, ch)
		close(ch)
	}
	s.mu.Unlock()
}

func (s *Service) poll(session *loginSession) {
	interval := 5 * time.Second
	deadline := time.NewTimer(15 * time.Minute)
	defer deadline.Stop()

	oauthCfg := auth.OpenAIOAuthConfig()

	for {
		select {
		case <-deadline.C:
			s.finishSession(session, "error", "OpenAI OAuth 登录超时")
			return
		case <-time.After(interval):
			cred, err := auth.PollDeviceCodeOnce(oauthCfg, session.Info.DeviceAuthID, session.Info.UserCode)
			if err != nil {
				continue
			}
			if cred != nil {
				if err := auth.SetCredential("openai", cred); err != nil {
					s.finishSession(session, "error", err.Error())
					return
				}
				s.finishSession(session, "success", "")
				return
			}
		}
	}
}

func (s *Service) finishSession(session *loginSession, status, errMsg string) {
	s.mu.Lock()
	if s.session != session {
		s.mu.Unlock()
		return
	}
	s.session.Status = status
	s.session.Error = errMsg
	s.session.Done = true
	current := s.currentStatusLocked()
	s.mu.Unlock()
	s.broadcast(current)
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
		Provider: "openai",
		Status:   "idle",
	}

	cred, err := auth.GetCredential("openai")
	if err == nil && cred != nil {
		status.AuthMethod = cred.AuthMethod
		status.AccountID = cred.AccountID
		status.Email = cred.Email
		if !cred.IsExpired() || cred.RefreshToken != "" {
			status.Connected = true
			status.Status = "connected"
		}
	}

	if s.session != nil {
		switch s.session.Status {
		case "pending":
			status.Status = "pending"
			status.PendingDevice = &PendingDevice{
				UserCode:          s.session.Info.UserCode,
				VerifyURL:         s.session.Info.VerifyURL,
				VerifyURLComplete: s.session.Info.VerifyURL,
				ExpiresIn:         900,
			}
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
