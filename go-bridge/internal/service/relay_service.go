package service

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/domain"
	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/opencode"
	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/ports"
)

type relayCacheEntry struct {
	Text    string
	Final   bool
	Updated time.Time
}

type RelayService struct {
	logger        *slog.Logger
	opencode      *opencode.Client
	repo          ports.SessionLinkRepository
	telegram      ports.TelegramClient
	mode          domain.RelayMode
	fallback      bool
	fallbackDelay time.Duration

	mu    sync.RWMutex
	cache map[string]relayCacheEntry
}

func NewRelayService(
	logger *slog.Logger,
	opencodeClient *opencode.Client,
	repo ports.SessionLinkRepository,
	telegram ports.TelegramClient,
	mode string,
	fallback bool,
	fallbackDelayMs int,
) *RelayService {
	resolvedMode := domain.RelayModeLast
	if strings.EqualFold(mode, string(domain.RelayModeFinal)) {
		resolvedMode = domain.RelayModeFinal
	}
	return &RelayService{
		logger:        logger,
		opencode:      opencodeClient,
		repo:          repo,
		telegram:      telegram,
		mode:          resolvedMode,
		fallback:      fallback,
		fallbackDelay: time.Duration(fallbackDelayMs) * time.Millisecond,
		cache:         make(map[string]relayCacheEntry),
	}
}

func (s *RelayService) Run(ctx context.Context) error {
	events, errs := s.opencode.StreamEvents(ctx)
	for {
		select {
		case <-ctx.Done():
			return nil
		case err, ok := <-errs:
			if !ok {
				continue
			}
			if err != nil {
				return err
			}
		case event, ok := <-events:
			if !ok {
				return nil
			}
			s.handleEvent(ctx, event)
		}
	}
}

func (s *RelayService) handleEvent(ctx context.Context, event opencode.Event) {
	if event.SessionID == "" {
		return
	}

	switch event.Type {
	case "message.updated":
		s.updateCache(event)
	case "session.idle":
		s.onSessionIdle(ctx, event.SessionID)
	}
}

func (s *RelayService) updateCache(event opencode.Event) {
	if strings.TrimSpace(event.Text) == "" {
		return
	}
	s.mu.Lock()
	s.cache[event.SessionID] = relayCacheEntry{Text: event.Text, Final: event.Final, Updated: time.Now()}
	s.mu.Unlock()
}

func (s *RelayService) onSessionIdle(ctx context.Context, sessionID string) {
	if s.mode == domain.RelayModeLast {
		sendText := s.cachedText(sessionID)
		if sendText == "" {
			sendText = s.fetchFinalText(ctx, sessionID)
		}
		s.dispatch(ctx, sessionID, sendText)
		return
	}

	entry, ok := s.cachedEntry(sessionID)
	if ok && entry.Final && strings.TrimSpace(entry.Text) != "" {
		s.dispatch(ctx, sessionID, entry.Text)
		return
	}

	if !s.fallback {
		return
	}

	timer := time.NewTimer(s.fallbackDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}

	sendText := s.cachedText(sessionID)
	if sendText == "" {
		sendText = s.fetchFinalText(ctx, sessionID)
	}
	s.dispatch(ctx, sessionID, sendText)
}

func (s *RelayService) dispatch(ctx context.Context, sessionID string, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	recipients, err := s.repo.FindRecipientsBySession(ctx, sessionID)
	if err != nil {
		s.logger.Error("relay recipients lookup failed", "session_id", sessionID, "error", err)
		return
	}
	for _, recipient := range recipients {
		if err := s.telegram.SendMessage(ctx, recipient.TelegramChatID, text); err != nil {
			s.logger.Error("relay telegram send failed", "chat_id", recipient.TelegramChatID, "error", err)
		}
	}
	s.mu.Lock()
	delete(s.cache, sessionID)
	s.mu.Unlock()
}

func (s *RelayService) cachedText(sessionID string) string {
	entry, ok := s.cachedEntry(sessionID)
	if !ok {
		return ""
	}
	return entry.Text
}

func (s *RelayService) cachedEntry(sessionID string) (relayCacheEntry, bool) {
	s.mu.RLock()
	entry, ok := s.cache[sessionID]
	s.mu.RUnlock()
	return entry, ok
}

func (s *RelayService) fetchFinalText(ctx context.Context, sessionID string) string {
	text, err := s.opencode.GetLastAssistantMessage(ctx, sessionID)
	if err != nil {
		s.logger.Error("fetch final text failed", "session_id", sessionID, "error", err)
		return ""
	}
	return strings.TrimSpace(text)
}
