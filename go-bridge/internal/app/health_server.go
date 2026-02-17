package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/config"
	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/opencode"
	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/service"
	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/telegram"
)

var ErrServerClosed = http.ErrServerClosed

type HealthServer struct {
	cfg        config.Config
	logger     *slog.Logger
	httpServer *http.Server
	startedAt  time.Time
	resolveFn  func(context.Context, []string) ResolveResponse
	controlSvc *service.ControlService
}

type ResolveResponse struct {
	Resolved   []ResolveItem `json:"resolved"`
	Unresolved []ResolveItem `json:"unresolved"`
}

type ResolveItem struct {
	Username string `json:"username"`
	UserID   int64  `json:"userId,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type serviceCheck struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type healthResponse struct {
	UptimeSeconds int64        `json:"uptimeSeconds"`
	OpenCode      serviceCheck `json:"opencode"`
	Telegram      serviceCheck `json:"telegram"`
	Relay         struct {
		Mode            string `json:"mode"`
		FallbackEnabled bool   `json:"fallbackEnabled"`
		FallbackDelayMs int    `json:"fallbackDelayMs"`
	} `json:"relay"`
}

func NewHealthServer(cfg config.Config, logger *slog.Logger, resolveFn func(context.Context, []string) ResolveResponse) *HealthServer {
	server := &HealthServer{cfg: cfg, logger: logger, startedAt: time.Now(), resolveFn: resolveFn}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", server.healthHandler)
	mux.HandleFunc("/resolve", server.resolveHandler)
	mux.HandleFunc("/command/status", server.commandStatusHandler)
	mux.HandleFunc("/command/session/get", server.commandSessionGetHandler)
	mux.HandleFunc("/command/session/list", server.commandSessionListHandler)
	mux.HandleFunc("/command/session/use", server.commandSessionUseHandler)
	mux.HandleFunc("/command/session/new", server.commandSessionNewHandler)
	mux.HandleFunc("/command/models/list", server.commandModelsListHandler)
	mux.HandleFunc("/command/models/set", server.commandModelsSetHandler)
	mux.HandleFunc("/command/models/clear", server.commandModelsClearHandler)
	mux.HandleFunc("/command/compact", server.commandCompactHandler)
	mux.HandleFunc("/command/allow", server.commandAllowHandler)
	mux.HandleFunc("/command/deny", server.commandDenyHandler)
	mux.HandleFunc("/command/access/list", server.commandAccessListHandler)

	server.httpServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HealthPort),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return server
}

func (s *HealthServer) SetControlService(control *service.ControlService) {
	s.controlSvc = control
}

func (s *HealthServer) resolveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.resolveFn == nil {
		http.Error(w, "resolve service unavailable", http.StatusServiceUnavailable)
		return
	}

	var payload struct {
		Usernames []string `json:"usernames"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	if len(payload.Usernames) == 0 {
		http.Error(w, "usernames is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	result := s.resolveFn(ctx, payload.Usernames)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		s.logger.Error("encode resolve response failed", "error", err)
	}
}

func (s *HealthServer) commandStatusHandler(w http.ResponseWriter, r *http.Request) {
	chatID, userID, ok := s.requireChatAndUser(w, r)
	if !ok {
		return
	}
	report, err := s.controlSvc.Status(r.Context(), chatID, userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.writeJSON(w, http.StatusOK, report)
}

func (s *HealthServer) commandSessionGetHandler(w http.ResponseWriter, r *http.Request) {
	chatID, userID, ok := s.requireChatAndUser(w, r)
	if !ok {
		return
	}
	sessionID, err := s.controlSvc.SessionCurrent(r.Context(), chatID, userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"sessionId": sessionID})
}

func (s *HealthServer) commandSessionListHandler(w http.ResponseWriter, r *http.Request) {
	chatID, userID, ok := s.requireChatAndUser(w, r)
	if !ok {
		return
	}
	list, err := s.controlSvc.SessionList(r.Context(), chatID, userID, 5)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"sessions": list})
}

func (s *HealthServer) commandSessionUseHandler(w http.ResponseWriter, r *http.Request) {
	payload, ok := s.parseChatUserPayload(w, r)
	if !ok {
		return
	}
	sessionID, ok := payload["sessionId"].(string)
	if !ok || strings.TrimSpace(sessionID) == "" {
		http.Error(w, "sessionId is required", http.StatusBadRequest)
		return
	}
	if err := s.controlSvc.SessionUse(r.Context(), payload.chatID(), payload.userID(), sessionID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"message": "session updated", "sessionId": sessionID})
}

func (s *HealthServer) commandSessionNewHandler(w http.ResponseWriter, r *http.Request) {
	chatID, userID, ok := s.requireChatAndUser(w, r)
	if !ok {
		return
	}
	defaultID, err := s.controlSvc.SessionNew(r.Context(), chatID, userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"message": "session reset", "defaultSessionId": defaultID})
}

func (s *HealthServer) commandModelsListHandler(w http.ResponseWriter, r *http.Request) {
	models, err := s.controlSvc.ModelsList(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

func (s *HealthServer) commandModelsSetHandler(w http.ResponseWriter, r *http.Request) {
	payload, ok := s.parseChatUserPayload(w, r)
	if !ok {
		return
	}
	model, ok := payload["model"].(string)
	if !ok || strings.TrimSpace(model) == "" {
		http.Error(w, "model is required", http.StatusBadRequest)
		return
	}
	sessionID, err := s.controlSvc.ModelsSet(r.Context(), payload.chatID(), payload.userID(), model)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"message": "model set", "sessionId": sessionID, "model": model})
}

func (s *HealthServer) commandModelsClearHandler(w http.ResponseWriter, r *http.Request) {
	chatID, userID, ok := s.requireChatAndUser(w, r)
	if !ok {
		return
	}
	sessionID, err := s.controlSvc.ModelsClear(r.Context(), chatID, userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"message": "model cleared", "sessionId": sessionID})
}

func (s *HealthServer) commandCompactHandler(w http.ResponseWriter, r *http.Request) {
	chatID, userID, ok := s.requireChatAndUser(w, r)
	if !ok {
		return
	}
	sessionID, err := s.controlSvc.Compact(r.Context(), chatID, userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"message": "session compacted", "sessionId": sessionID})
}

func (s *HealthServer) commandAllowHandler(w http.ResponseWriter, r *http.Request) {
	targetID, ok := s.parseTargetUserID(w, r)
	if !ok {
		return
	}
	if err := s.controlSvc.Allow(r.Context(), targetID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"message": "user allowed", "targetUserId": targetID})
}

func (s *HealthServer) commandDenyHandler(w http.ResponseWriter, r *http.Request) {
	targetID, ok := s.parseTargetUserID(w, r)
	if !ok {
		return
	}
	if err := s.controlSvc.Deny(r.Context(), targetID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"message": "user removed", "targetUserId": targetID})
}

func (s *HealthServer) commandAccessListHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.controlSvc == nil {
		http.Error(w, "control service unavailable", http.StatusServiceUnavailable)
		return
	}
	list, err := s.controlSvc.AccessList(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.writeJSON(w, http.StatusOK, list)
}

type chatUserPayload map[string]any

func (p chatUserPayload) chatID() int64 {
	v, _ := parseInt64Any(p["chatId"])
	return v
}

func (p chatUserPayload) userID() int64 {
	v, _ := parseInt64Any(p["userId"])
	return v
}

func (s *HealthServer) parseChatUserPayload(w http.ResponseWriter, r *http.Request) (chatUserPayload, bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return nil, false
	}
	if s.controlSvc == nil {
		http.Error(w, "control service unavailable", http.StatusServiceUnavailable)
		return nil, false
	}
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return nil, false
	}
	chatID, chatOK := parseInt64Any(payload["chatId"])
	userID, userOK := parseInt64Any(payload["userId"])
	if !chatOK || !userOK || chatID == 0 || userID == 0 {
		http.Error(w, "chatId and userId are required", http.StatusBadRequest)
		return nil, false
	}
	return chatUserPayload(payload), true
}

func (s *HealthServer) requireChatAndUser(w http.ResponseWriter, r *http.Request) (int64, int64, bool) {
	payload, ok := s.parseChatUserPayload(w, r)
	if !ok {
		return 0, 0, false
	}
	return payload.chatID(), payload.userID(), true
}

func (s *HealthServer) parseTargetUserID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return 0, false
	}
	if s.controlSvc == nil {
		http.Error(w, "control service unavailable", http.StatusServiceUnavailable)
		return 0, false
	}
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return 0, false
	}
	targetID, ok := parseInt64Any(payload["targetUserId"])
	if !ok || targetID == 0 {
		http.Error(w, "targetUserId is required", http.StatusBadRequest)
		return 0, false
	}
	return targetID, true
}

func parseInt64Any(value any) (int64, bool) {
	switch v := value.(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case int:
		return int64(v), true
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func (s *HealthServer) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		s.logger.Error("encode json response failed", "error", err)
	}
}

func (s *HealthServer) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

func (s *HealthServer) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *HealthServer) healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	res := healthResponse{UptimeSeconds: int64(time.Since(s.startedAt).Seconds())}
	res.Relay.Mode = s.cfg.RelayMode
	res.Relay.FallbackEnabled = s.cfg.RelayFallback
	res.Relay.FallbackDelayMs = s.cfg.RelayFallbackDelayMs

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		err := opencode.CheckConnectivity(ctx, s.cfg)
		res.OpenCode = checkFromErr(err)
	}()

	go func() {
		defer wg.Done()
		err := telegram.CheckConnectivity(ctx, s.cfg.BotToken, s.cfg.OpenCodeTimeout)
		res.Telegram = checkFromErr(err)
	}()

	wg.Wait()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(res); err != nil {
		s.logger.Error("encode health response failed", "error", err)
	}
}

func checkFromErr(err error) serviceCheck {
	if err == nil {
		return serviceCheck{OK: true}
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		msg = "unknown error"
	}
	return serviceCheck{OK: false, Error: msg}
}

func IsServerClosed(err error) bool {
	return errors.Is(err, ErrServerClosed)
}
