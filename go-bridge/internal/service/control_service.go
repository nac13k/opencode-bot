package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/opencode"
	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/ports"
)

type AccessList struct {
	Admins  []int64 `json:"admins"`
	Allowed []int64 `json:"allowed"`
}

type ControlService struct {
	authz      ports.AuthzRepository
	sessions   *SessionLinkService
	models     ports.SessionModelRepository
	opencode   *opencode.Client
	listLimit  int
	listSource string
}

func NewControlService(
	authz ports.AuthzRepository,
	sessions *SessionLinkService,
	models ports.SessionModelRepository,
	opencodeClient *opencode.Client,
	sessionsListLimit int,
	sessionsSource string,
) *ControlService {
	if sessionsListLimit <= 0 {
		sessionsListLimit = 5
	}
	resolvedSource := strings.ToLower(strings.TrimSpace(sessionsSource))
	if resolvedSource == "" {
		resolvedSource = "both"
	}
	return &ControlService{authz: authz, sessions: sessions, models: models, opencode: opencodeClient, listLimit: sessionsListLimit, listSource: resolvedSource}
}

func (s *ControlService) Status(ctx context.Context, chatID int64, userID int64) (opencode.StatusReport, error) {
	sessionID, err := s.sessions.GetSession(ctx, chatID, userID)
	if err != nil {
		return opencode.StatusReport{}, err
	}
	return s.opencode.GetStatus(ctx, sessionID)
}

func (s *ControlService) SessionCurrent(ctx context.Context, chatID int64, userID int64) (string, error) {
	return s.sessions.GetSession(ctx, chatID, userID)
}

func (s *ControlService) SessionList(ctx context.Context, chatID int64, userID int64) ([]opencode.SessionSummary, error) {
	current, err := s.sessions.GetSession(ctx, chatID, userID)
	if err != nil {
		return nil, err
	}
	return s.opencode.ListSessionsWithCurrent(ctx, current, s.listLimit, s.listSource)
}

func (s *ControlService) SessionUse(ctx context.Context, chatID int64, userID int64, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("session id is required")
	}
	return s.sessions.SetSession(ctx, chatID, userID, strings.TrimSpace(sessionID))
}

func (s *ControlService) SessionNew(ctx context.Context, chatID int64, userID int64) (string, error) {
	if err := s.sessions.ClearSession(ctx, chatID, userID); err != nil {
		return "", err
	}
	if defaultID := s.sessions.DefaultSessionID(); defaultID != "" {
		if err := s.sessions.SetSession(ctx, chatID, userID, defaultID); err != nil {
			return "", err
		}
		return defaultID, nil
	}
	return "", nil
}

func (s *ControlService) ModelsList(ctx context.Context) ([]opencode.ModelInfo, error) {
	return s.opencode.ListFavoriteModels(ctx)
}

func (s *ControlService) ModelsSet(ctx context.Context, chatID int64, userID int64, model string) (string, error) {
	sessionID, err := s.sessions.GetSession(ctx, chatID, userID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(sessionID) == "" {
		return "", fmt.Errorf("no active session")
	}
	if err := s.models.UpsertSessionModel(ctx, sessionID, strings.TrimSpace(model)); err != nil {
		return "", err
	}
	return sessionID, nil
}

func (s *ControlService) ModelsClear(ctx context.Context, chatID int64, userID int64) (string, error) {
	sessionID, err := s.sessions.GetSession(ctx, chatID, userID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(sessionID) == "" {
		return "", fmt.Errorf("no active session")
	}
	if err := s.models.ClearSessionModel(ctx, sessionID); err != nil {
		return "", err
	}
	return sessionID, nil
}

func (s *ControlService) Compact(ctx context.Context, chatID int64, userID int64) (string, error) {
	sessionID, err := s.sessions.GetSession(ctx, chatID, userID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(sessionID) == "" {
		return "", fmt.Errorf("no active session")
	}
	if err := s.opencode.CompactSession(ctx, sessionID); err != nil {
		return "", err
	}
	return sessionID, nil
}

func (s *ControlService) Allow(ctx context.Context, targetUserID int64) error {
	return s.authz.UpsertAllowed(ctx, targetUserID)
}

func (s *ControlService) Deny(ctx context.Context, targetUserID int64) error {
	return s.authz.RemoveAllowed(ctx, targetUserID)
}

func (s *ControlService) AccessList(ctx context.Context) (AccessList, error) {
	admins, err := s.authz.ListAdmins(ctx)
	if err != nil {
		return AccessList{}, err
	}
	allowed, err := s.authz.ListAllowed(ctx)
	if err != nil {
		return AccessList{}, err
	}
	return AccessList{Admins: admins, Allowed: allowed}, nil
}
