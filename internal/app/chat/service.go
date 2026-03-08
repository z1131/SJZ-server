package chat

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/internal/uiapi/dto"
	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/session"
)

const defaultSessionTitle = "new"
const desktopSessionPrefix = "agent:main:chat:"

type Service struct {
	configPath string
}

func NewService() *Service {
	return &Service{configPath: getConfigPath()}
}

func NewServiceWithPath(path string) *Service {
	return &Service{configPath: path}
}

func (s *Service) ListSessions() (dto.ChatSessionsResponse, error) {
	cfg, err := config.LoadConfig(s.configPath)
	if err != nil {
		return dto.ChatSessionsResponse{}, fmt.Errorf("loading config: %w", err)
	}

	sm := session.NewSessionManager(s.sessionsDir(cfg))
	items := sm.ListSessions()
	sort.Slice(items, func(i, j int) bool {
		return items[i].Updated.After(items[j].Updated)
	})

	resp := dto.ChatSessionsResponse{
		Sessions: make([]dto.ChatSessionListItem, 0, len(items)),
	}
	for _, item := range items {
		if !isDesktopSessionKey(item.Key) {
			continue
		}
		resp.Sessions = append(resp.Sessions, dto.ChatSessionListItem{
			ID:           item.Key,
			Title:        sessionTitle(item.Title),
			MessageCount: item.MessageCount,
			UpdatedAt:    item.Updated.Format(time.RFC3339),
		})
	}

	return resp, nil
}

func (s *Service) CreateSession(title string) (dto.CreateChatSessionResponse, error) {
	cfg, err := config.LoadConfig(s.configPath)
	if err != nil {
		return dto.CreateChatSessionResponse{}, fmt.Errorf("loading config: %w", err)
	}

	sm := session.NewSessionManager(s.sessionsDir(cfg))
	sessionKey := fmt.Sprintf("%s%d", desktopSessionPrefix, time.Now().UnixNano())
	sm.GetOrCreate(sessionKey)
	sm.SetTitle(sessionKey, sessionTitle(title))
	if err := sm.Save(sessionKey); err != nil {
		return dto.CreateChatSessionResponse{}, fmt.Errorf("saving session: %w", err)
	}

	info := sm.ListSessions()
	for _, item := range info {
		if item.Key != sessionKey {
			continue
		}
		return dto.CreateChatSessionResponse{
			Session: dto.ChatSessionListItem{
				ID:           item.Key,
				Title:        sessionTitle(item.Title),
				MessageCount: item.MessageCount,
				UpdatedAt:    item.Updated.Format(time.RFC3339),
			},
		}, nil
	}

	return dto.CreateChatSessionResponse{
		Session: dto.ChatSessionListItem{
			ID:           sessionKey,
			Title:        sessionTitle(title),
			MessageCount: 0,
			UpdatedAt:    time.Now().Format(time.RFC3339),
		},
	}, nil
}

func (s *Service) GetMessages(sessionID string) (dto.ChatMessagesResponse, error) {
	if !isDesktopSessionKey(sessionID) {
		return dto.ChatMessagesResponse{}, fmt.Errorf("session is not a desktop chat session")
	}

	cfg, err := config.LoadConfig(s.configPath)
	if err != nil {
		return dto.ChatMessagesResponse{}, fmt.Errorf("loading config: %w", err)
	}

	sm := session.NewSessionManager(s.sessionsDir(cfg))
	history := sm.GetHistory(sessionID)
	return dto.ChatMessagesResponse{
		Session: dto.ChatSessionSummary{
			ID:    sessionID,
			Title: sessionTitle(sm.GetTitle(sessionID)),
		},
		Messages: mapMessages(sessionID, history),
	}, nil
}

func (s *Service) SendMessage(ctx context.Context, sessionID, content string) (dto.SendChatMessageResponse, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return dto.SendChatMessageResponse{}, fmt.Errorf("content is required")
	}
	if !isDesktopSessionKey(sessionID) {
		return dto.SendChatMessageResponse{}, fmt.Errorf("session is not a desktop chat session")
	}

	cfg, err := config.LoadConfig(s.configPath)
	if err != nil {
		return dto.SendChatMessageResponse{}, fmt.Errorf("loading config: %w", err)
	}

	sm := session.NewSessionManager(s.sessionsDir(cfg))
	sm.GetOrCreate(sessionID)
	if sm.GetTitle(sessionID) == "" {
		sm.SetTitle(sessionID, defaultSessionTitle)
		if err := sm.Save(sessionID); err != nil {
			return dto.SendChatMessageResponse{}, fmt.Errorf("saving session: %w", err)
		}
	}

	provider, _, err := providers.CreateProvider(cfg)
	if err != nil {
		return dto.SendChatMessageResponse{}, fmt.Errorf("creating provider: %w", err)
	}

	loop := agent.NewAgentLoop(cfg, bus.NewMessageBus(), provider)
	if _, err := loop.ProcessDirect(ctx, content, sessionID); err != nil {
		return dto.SendChatMessageResponse{}, err
	}

	sm = session.NewSessionManager(s.sessionsDir(cfg))
	title := sm.GetTitle(sessionID)
	if title == "" || title == defaultSessionTitle {
		sm.SetTitle(sessionID, summarizeTitle(content))
		if err := sm.Save(sessionID); err != nil {
			return dto.SendChatMessageResponse{}, fmt.Errorf("saving session title: %w", err)
		}
	}

	history := sm.GetHistory(sessionID)
	return dto.SendChatMessageResponse{
		Session: dto.ChatSessionSummary{
			ID:    sessionID,
			Title: sessionTitle(sm.GetTitle(sessionID)),
		},
		Messages: mapMessages(sessionID, history),
	}, nil
}

func (s *Service) sessionsDir(cfg *config.Config) string {
	return filepath.Join(cfg.WorkspacePath(), "sessions")
}

func mapMessages(sessionID string, history []providers.Message) []dto.ChatMessageItem {
	items := make([]dto.ChatMessageItem, 0, len(history))
	for i, msg := range history {
		items = append(items, dto.ChatMessageItem{
			ID:      fmt.Sprintf("%s:%d", sessionID, i),
			Role:    msg.Role,
			Content: msg.Content,
		})
	}
	return items
}

func sessionTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return defaultSessionTitle
	}
	return title
}

func summarizeTitle(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return defaultSessionTitle
	}

	runes := []rune(content)
	if len(runes) > 24 {
		return string(runes[:24])
	}
	return content
}

func isDesktopSessionKey(key string) bool {
	return strings.HasPrefix(strings.TrimSpace(key), desktopSessionPrefix)
}

func getConfigPath() string {
	if path := os.Getenv("PICOCLAW_CONFIG"); path != "" {
		return path
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".picoclaw", "config.json")
}
