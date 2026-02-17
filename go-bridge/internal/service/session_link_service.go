package service

import (
	"context"

	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/ports"
)

type SessionLinkService struct {
	repo             ports.SessionLinkRepository
	defaultSessionID string
}

func NewSessionLinkService(repo ports.SessionLinkRepository, defaultSessionID string) *SessionLinkService {
	return &SessionLinkService{repo: repo, defaultSessionID: defaultSessionID}
}

func (s *SessionLinkService) DefaultSessionID() string {
	return s.defaultSessionID
}

func (s *SessionLinkService) GetSession(ctx context.Context, chatID int64, userID int64) (string, error) {
	current, ok, err := s.repo.GetSessionLink(ctx, chatID, userID)
	if err != nil {
		return "", err
	}
	if ok {
		return current, nil
	}
	if s.defaultSessionID == "" {
		return "", nil
	}
	if err := s.repo.UpsertSessionLink(ctx, chatID, userID, s.defaultSessionID); err != nil {
		return "", err
	}
	return s.defaultSessionID, nil
}

func (s *SessionLinkService) SetSession(ctx context.Context, chatID int64, userID int64, sessionID string) error {
	return s.repo.UpsertSessionLink(ctx, chatID, userID, sessionID)
}

func (s *SessionLinkService) ClearSession(ctx context.Context, chatID int64, userID int64) error {
	return s.repo.ClearSessionLink(ctx, chatID, userID)
}
