package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/opencode"
	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/ports"
	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/telegram"
)

var sessionIDPattern = regexp.MustCompile(`^ses_[A-Za-z0-9]+$`)

type InteractiveTelegramClient interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
	SendChatAction(ctx context.Context, chatID int64, action string) error
	SendMessageWithInlineKeyboard(ctx context.Context, chatID int64, text string, rows [][]telegram.InlineKeyboardButton) error
	AnswerCallbackQuery(ctx context.Context, callbackQueryID string, text string) error
}

type BridgeService struct {
	logger      *slog.Logger
	opencode    *opencode.Client
	telegramAPI InteractiveTelegramClient
	authzRepo   ports.AuthzRepository
	models      ports.SessionModelRepository
	sessions    *SessionLinkService
	queue       *KeyedQueue
	sessionsCfg struct {
		limit      int
		source     string
		showIDList bool
	}
}

func NewBridgeService(
	logger *slog.Logger,
	opencodeClient *opencode.Client,
	telegramClient InteractiveTelegramClient,
	authzRepo ports.AuthzRepository,
	modelRepo ports.SessionModelRepository,
	sessions *SessionLinkService,
	sessionsListLimit int,
	sessionsSource string,
	sessionsShowIDList bool,
) *BridgeService {
	if sessionsListLimit <= 0 {
		sessionsListLimit = 5
	}
	return &BridgeService{
		logger:      logger,
		opencode:    opencodeClient,
		telegramAPI: telegramClient,
		authzRepo:   authzRepo,
		models:      modelRepo,
		sessions:    sessions,
		queue:       NewKeyedQueue(),
		sessionsCfg: struct {
			limit      int
			source     string
			showIDList bool
		}{
			limit:      sessionsListLimit,
			source:     strings.ToLower(strings.TrimSpace(sessionsSource)),
			showIDList: sessionsShowIDList,
		},
	}
}

func (s *BridgeService) HandleUpdate(ctx context.Context, update telegram.Update) {
	if update.CallbackQuery != nil {
		s.handleCallbackQuery(ctx, update.CallbackQuery)
		return
	}

	if update.Message == nil {
		return
	}
	message := update.Message
	if message.From.ID == 0 || message.Chat.ID == 0 {
		return
	}

	text := strings.TrimSpace(message.Text)
	if text == "" {
		return
	}

	if strings.HasPrefix(text, "/") {
		s.handleCommand(ctx, *message)
		return
	}

	allowed, err := s.authzRepo.IsAllowed(ctx, message.From.ID)
	if err != nil {
		s.logger.Error("auth check failed", "error", err)
		return
	}
	if !allowed {
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "No autorizado. Pide acceso al admin con tu userId.")
		return
	}

	queueKey := fmt.Sprintf("%d:%d", message.Chat.ID, message.From.ID)
	err = s.queue.Run(ctx, queueKey, func(ctx context.Context) error {
		sessionID, err := s.sessions.GetSession(ctx, message.Chat.ID, message.From.ID)
		if err != nil {
			return err
		}
		previousSnapshot := opencode.AssistantSnapshot{}
		if sessionID != "" {
			if snapshot, snapshotErr := s.opencode.GetAssistantSnapshot(ctx, sessionID); snapshotErr == nil {
				previousSnapshot = snapshot
			}
		}
		model := ""
		if sessionID != "" {
			storedModel, ok, modelErr := s.models.GetSessionModel(ctx, sessionID)
			if modelErr != nil {
				return modelErr
			}
			if ok {
				model = storedModel
			}
		}
		newSessionID, err := s.opencode.RunPrompt(ctx, text, sessionID, model)
		if err != nil {
			return err
		}
		if newSessionID != "" && newSessionID != sessionID {
			if err := s.sessions.SetSession(ctx, message.Chat.ID, message.From.ID, newSessionID); err != nil {
				return err
			}
			if strings.TrimSpace(model) != "" {
				if err := s.models.UpsertSessionModel(ctx, newSessionID, model); err != nil {
					return err
				}
			}
		}
		if err := s.telegramAPI.SendMessage(ctx, message.Chat.ID, "Procesando solicitud..."); err != nil {
			return err
		}

		responseText, waitErr := s.waitForAssistantResponse(ctx, message.Chat.ID, newSessionID, previousSnapshot)
		if waitErr != nil {
			return waitErr
		}
		if strings.TrimSpace(responseText) == "" {
			return s.telegramAPI.SendMessage(ctx, message.Chat.ID, "OpenCode no devolvio texto en esta respuesta.")
		}
		return s.telegramAPI.SendMessage(ctx, message.Chat.ID, responseText)
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			s.logger.Info("prompt canceled during shutdown", "chat_id", message.Chat.ID, "user_id", message.From.ID)
			return
		}
		s.logger.Error("handle prompt failed", "error", err, "chat_id", message.Chat.ID, "user_id", message.From.ID)
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, userFacingOpenCodeError(err))
	}
}

func (s *BridgeService) handleCallbackQuery(ctx context.Context, query *telegram.CallbackQuery) {
	if query == nil || query.Message == nil {
		return
	}
	data := strings.TrimSpace(query.Data)
	if data == "" {
		return
	}
	if !strings.HasPrefix(data, "session_use:") {
		_ = s.telegramAPI.AnswerCallbackQuery(ctx, query.ID, "Accion no soportada")
		return
	}

	sessionID := strings.TrimPrefix(data, "session_use:")
	if !sessionIDPattern.MatchString(sessionID) {
		_ = s.telegramAPI.AnswerCallbackQuery(ctx, query.ID, "Sesion invalida")
		return
	}

	message := telegram.Message{From: query.From, Chat: query.Message.Chat}
	if !s.requireAllowed(ctx, message) {
		_ = s.telegramAPI.AnswerCallbackQuery(ctx, query.ID, "No autorizado")
		return
	}

	if err := s.sessions.SetSession(ctx, query.Message.Chat.ID, query.From.ID, sessionID); err != nil {
		s.logger.Error("set session from callback failed", "error", err, "session_id", sessionID)
		_ = s.telegramAPI.AnswerCallbackQuery(ctx, query.ID, "No se pudo cambiar sesion")
		return
	}

	_ = s.telegramAPI.AnswerCallbackQuery(ctx, query.ID, "Sesion seleccionada")
	_ = s.telegramAPI.SendMessage(ctx, query.Message.Chat.ID, "Sesion seleccionada: "+sessionID)
}

func (s *BridgeService) handleCommand(ctx context.Context, message telegram.Message) {
	fields := strings.Fields(strings.TrimSpace(message.Text))
	if len(fields) == 0 {
		return
	}
	name := strings.TrimPrefix(fields[0], "/")
	args := fields[1:]

	switch name {
	case "start":
		s.handleStart(ctx, message)
	case "status":
		s.handleStatus(ctx, message)
	case "compact":
		s.handleCompact(ctx, message)
	case "session":
		s.handleSession(ctx, message, args)
	case "sessions":
		s.handleSessions(ctx, message)
	case "models":
		s.handleModels(ctx, message, args)
	case "allow":
		s.handleAllow(ctx, message, args)
	case "deny":
		s.handleDeny(ctx, message, args)
	case "list":
		s.handleList(ctx, message)
	default:
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "Comando no soportado aun. Usa /start /status /session /sessions /compact /models /allow /deny /list.")
	}
}

func (s *BridgeService) handleStart(ctx context.Context, message telegram.Message) {
	allowed, err := s.authzRepo.IsAllowed(ctx, message.From.ID)
	if err != nil {
		s.logger.Error("auth check failed on start", "error", err)
		return
	}
	if allowed {
		_, _ = s.sessions.GetSession(ctx, message.Chat.ID, message.From.ID)
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "Bot listo. Puedes enviar instrucciones para OpenCode.")
		return
	}
	_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "No autorizado. Pide a un admin que te agregue por userId.")
}

func (s *BridgeService) handleStatus(ctx context.Context, message telegram.Message) {
	if !s.requireAllowed(ctx, message) {
		return
	}
	sessionID, err := s.sessions.GetSession(ctx, message.Chat.ID, message.From.ID)
	if err != nil {
		s.logger.Error("get session for status failed", "error", err)
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "No se pudo obtener la sesion actual.")
		return
	}
	if sessionID == "" {
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "Sin sesion activa. Envia un mensaje para crear una sesion nueva.")
		return
	}

	report, err := s.opencode.GetStatus(ctx, sessionID)
	if err != nil {
		s.logger.Error("status request failed", "error", err)
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "No se pudo consultar status de OpenCode.")
		return
	}
	model := report.Model
	if strings.TrimSpace(model) == "" {
		model = "n/d"
	}
	status := report.Status
	if strings.TrimSpace(status) == "" {
		status = "unknown"
	}
	text := fmt.Sprintf("Status OpenCode\n• Sesion: %s\n• Estado: %s\n• Modelo: %s", sessionID, status, model)
	_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, text)
}

func (s *BridgeService) handleCompact(ctx context.Context, message telegram.Message) {
	if !s.requireAllowed(ctx, message) {
		return
	}
	sessionID, err := s.sessions.GetSession(ctx, message.Chat.ID, message.From.ID)
	if err != nil {
		s.logger.Error("get session for compact failed", "error", err)
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "No se pudo obtener la sesion actual.")
		return
	}
	if sessionID == "" {
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "No hay sesion activa para compactar.")
		return
	}
	if err := s.opencode.CompactSession(ctx, sessionID); err != nil {
		s.logger.Error("compact failed", "error", err, "session_id", sessionID)
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "No se pudo compactar la sesion.")
		return
	}
	_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "Sesion compactada.")
}

func (s *BridgeService) handleSessions(ctx context.Context, message telegram.Message) {
	if !s.requireAllowed(ctx, message) {
		return
	}
	current, err := s.sessions.GetSession(ctx, message.Chat.ID, message.From.ID)
	if err != nil {
		s.logger.Error("get session for sessions failed", "error", err)
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "No se pudo obtener la sesion actual.")
		return
	}
	list, err := s.opencode.ListSessionsWithCurrent(ctx, current, s.sessionsCfg.limit, s.sessionsCfg.source)
	if err != nil {
		s.logger.Error("list sessions failed", "error", err)
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "No se pudieron listar sesiones de OpenCode.")
		return
	}
	if len(list) == 0 {
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "No hay sesiones disponibles en OpenCode.")
		return
	}
	lines := make([]string, 0, len(list)+2)
	if s.sessionsCfg.showIDList {
		lines = append(lines, "Sesiones recientes:")
	} else {
		lines = append(lines, "Sesiones recientes (toca para seleccionar):")
	}
	buttons := make([][]telegram.InlineKeyboardButton, 0, len(list))
	for i, item := range list {
		suffix := ""
		if item.ID == current {
			suffix = " [actual]"
		}
		if s.sessionsCfg.showIDList {
			lines = append(lines, fmt.Sprintf("%d. %s (%s)%s", i+1, item.Title, item.ID, suffix))
		}

		buttonLabel := strings.TrimSpace(item.Title)
		timeLabel := formatSessionTimeLabel(item.Updated)
		if buttonLabel != "" {
			buttonLabel = timeLabel + " " + buttonLabel
		} else {
			buttonLabel = timeLabel
		}
		if len(buttonLabel) > 40 {
			buttonLabel = buttonLabel[:37] + "..."
		}
		if item.ID == current {
			buttonLabel = "* " + buttonLabel
		}
		buttons = append(buttons, []telegram.InlineKeyboardButton{{
			Text:         buttonLabel,
			CallbackData: "session_use:" + item.ID,
		}})
	}
	_ = s.telegramAPI.SendMessageWithInlineKeyboard(ctx, message.Chat.ID, strings.Join(lines, "\n"), buttons)
}

func (s *BridgeService) handleSession(ctx context.Context, message telegram.Message, args []string) {
	if !s.requireAllowed(ctx, message) {
		return
	}
	if len(args) == 0 {
		current, err := s.sessions.GetSession(ctx, message.Chat.ID, message.From.ID)
		if err != nil {
			s.logger.Error("get session failed", "error", err)
			_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "No se pudo obtener la sesion actual.")
			return
		}
		if current == "" {
			current = "(nueva en el proximo mensaje)"
		}
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "Sesion actual: "+current+"\nUso: /session list | /session use <ses_...> | /session new")
		return
	}

	switch args[0] {
	case "list":
		s.handleSessions(ctx, message)
	case "new":
		if err := s.sessions.ClearSession(ctx, message.Chat.ID, message.From.ID); err != nil {
			s.logger.Error("clear session failed", "error", err)
			_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "No se pudo reiniciar la sesion.")
			return
		}
		defaultSessionID := s.sessions.DefaultSessionID()
		if defaultSessionID != "" {
			if err := s.sessions.SetSession(ctx, message.Chat.ID, message.From.ID, defaultSessionID); err != nil {
				s.logger.Error("set default session failed", "error", err)
				_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "Sesion reiniciada, pero no se pudo aplicar la sesion default.")
				return
			}
			_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "Sesion reiniciada. Sesion default: "+defaultSessionID)
			return
		}
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "Sesion reiniciada. El proximo mensaje creara una sesion nueva.")
	case "use":
		if len(args) < 2 || !sessionIDPattern.MatchString(args[1]) {
			_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "Uso: /session use <ses_...>")
			return
		}
		if err := s.sessions.SetSession(ctx, message.Chat.ID, message.From.ID, args[1]); err != nil {
			s.logger.Error("set session failed", "error", err)
			_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "No se pudo cambiar la sesion.")
			return
		}
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "Sesion seleccionada: "+args[1])
	default:
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "Accion invalida. Usa /session list | /session use <ses_...> | /session new")
	}
}

func (s *BridgeService) handleModels(ctx context.Context, message telegram.Message, args []string) {
	if !s.requireAllowed(ctx, message) {
		return
	}

	if len(args) == 0 || args[0] == "list" {
		models, err := s.opencode.ListFavoriteModels(ctx)
		if err != nil {
			s.logger.Error("list models failed", "error", err)
			_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "No se pudieron listar modelos favoritos.")
			return
		}
		if len(models) == 0 {
			_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "No hay modelos favoritos en OpenCode.")
			return
		}
		lines := []string{"Modelos favoritos:"}
		for i, model := range models {
			name := strings.TrimSpace(model.Name)
			if name == "" {
				name = model.ID
			}
			lines = append(lines, fmt.Sprintf("%d. %s (%s)", i+1, name, model.ID))
		}
		lines = append(lines, "Usa /models set <model-id> o /models clear")
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, strings.Join(lines, "\n"))
		return
	}

	sessionID, err := s.sessions.GetSession(ctx, message.Chat.ID, message.From.ID)
	if err != nil {
		s.logger.Error("get session for models failed", "error", err)
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "No se pudo obtener la sesion actual.")
		return
	}
	if sessionID == "" {
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "No hay sesion activa. Envia un mensaje primero.")
		return
	}

	switch args[0] {
	case "set":
		if len(args) < 2 {
			_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "Uso: /models set <model-id>")
			return
		}
		modelID := strings.TrimSpace(args[1])
		if modelID == "" {
			_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "Uso: /models set <model-id>")
			return
		}
		if err := s.models.UpsertSessionModel(ctx, sessionID, modelID); err != nil {
			s.logger.Error("set session model failed", "error", err)
			_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "No se pudo establecer el modelo.")
			return
		}
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "Modelo seleccionado: "+modelID)
	case "clear":
		if err := s.models.ClearSessionModel(ctx, sessionID); err != nil {
			s.logger.Error("clear session model failed", "error", err)
			_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "No se pudo limpiar el modelo.")
			return
		}
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "Modelo limpiado. Se usara el default de OpenCode.")
	default:
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "Uso: /models list | /models set <model-id> | /models clear")
	}
}

func (s *BridgeService) requireAllowed(ctx context.Context, message telegram.Message) bool {
	allowed, err := s.authzRepo.IsAllowed(ctx, message.From.ID)
	if err != nil {
		s.logger.Error("auth check failed", "error", err)
		return false
	}
	if !allowed {
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "No autorizado.")
		return false
	}
	return true
}

func (s *BridgeService) requireAdmin(ctx context.Context, message telegram.Message) bool {
	isAdmin, err := s.authzRepo.IsAdmin(ctx, message.From.ID)
	if err != nil {
		s.logger.Error("admin check failed", "error", err)
		return false
	}
	if !isAdmin {
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "Comando solo para admins.")
		return false
	}
	return true
}

func (s *BridgeService) handleAllow(ctx context.Context, message telegram.Message, args []string) {
	if !s.requireAdmin(ctx, message) {
		return
	}
	if len(args) == 0 {
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "Uso: /allow <telegramUserId>")
		return
	}
	userID, err := parseTelegramID(args[0])
	if err != nil {
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "Uso: /allow <telegramUserId>")
		return
	}
	if err := s.authzRepo.UpsertAllowed(ctx, userID); err != nil {
		s.logger.Error("allow user failed", "error", err, "target_user_id", userID)
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "No se pudo agregar el usuario.")
		return
	}
	_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, fmt.Sprintf("Usuario permitido: %d", userID))
}

func (s *BridgeService) handleDeny(ctx context.Context, message telegram.Message, args []string) {
	if !s.requireAdmin(ctx, message) {
		return
	}
	if len(args) == 0 {
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "Uso: /deny <telegramUserId>")
		return
	}
	userID, err := parseTelegramID(args[0])
	if err != nil {
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "Uso: /deny <telegramUserId>")
		return
	}
	if err := s.authzRepo.RemoveAllowed(ctx, userID); err != nil {
		s.logger.Error("deny user failed", "error", err, "target_user_id", userID)
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "No se pudo remover el usuario.")
		return
	}
	_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, fmt.Sprintf("Usuario removido: %d", userID))
}

func (s *BridgeService) handleList(ctx context.Context, message telegram.Message) {
	if !s.requireAdmin(ctx, message) {
		return
	}
	admins, err := s.authzRepo.ListAdmins(ctx)
	if err != nil {
		s.logger.Error("list admins failed", "error", err)
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "No se pudo listar admins.")
		return
	}
	allowed, err := s.authzRepo.ListAllowed(ctx)
	if err != nil {
		s.logger.Error("list allowed failed", "error", err)
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "No se pudo listar usuarios permitidos.")
		return
	}
	adminText := formatIDList(admins)
	allowedText := formatIDList(allowed)
	text := "Acceso\n" +
		"• Admins: " + adminText + "\n" +
		"• Allowed: " + allowedText
	_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, text)
}

func parseTelegramID(value string) (int64, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, fmt.Errorf("empty id")
	}
	parsed, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return 0, err
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("invalid id")
	}
	return parsed, nil
}

func formatIDList(items []int64) string {
	if len(items) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, strconv.FormatInt(item, 10))
	}
	return strings.Join(parts, ", ")
}

func (s *BridgeService) waitForAssistantResponse(ctx context.Context, chatID int64, sessionID string, previous opencode.AssistantSnapshot) (string, error) {
	if strings.TrimSpace(sessionID) == "" {
		return "", fmt.Errorf("session id is empty")
	}

	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	stopTyping := make(chan struct{})
	defer close(stopTyping)

	go func() {
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopTyping:
				return
			case <-waitCtx.Done():
				return
			case <-ticker.C:
				_ = s.telegramAPI.SendChatAction(waitCtx, chatID, "typing")
			}
		}
	}()

	_ = s.telegramAPI.SendChatAction(waitCtx, chatID, "typing")

	lastState := "unknown"
	lastSnapshot := previous
	for {
		now, snapErr := s.opencode.GetAssistantSnapshot(waitCtx, sessionID)
		if snapErr == nil {
			lastSnapshot = now
			if now.Count > previous.Count && strings.TrimSpace(now.Last) != "" {
				return strings.TrimSpace(now.Last), nil
			}
			if strings.TrimSpace(now.Last) != "" && strings.TrimSpace(now.Last) != strings.TrimSpace(previous.Last) {
				return strings.TrimSpace(now.Last), nil
			}
		}

		state, stateErr := s.opencode.GetSessionState(waitCtx, sessionID)
		if stateErr == nil {
			lastState = state
		}
		if stateErr == nil && isErrorState(state) {
			if strings.TrimSpace(lastSnapshot.Last) != "" {
				return strings.TrimSpace(lastSnapshot.Last), nil
			}
			return "", fmt.Errorf("opencode session entered error state: %s", state)
		}
		if stateErr == nil && isIdleState(state) {
			snapshot, idleSnapErr := s.opencode.GetAssistantSnapshot(waitCtx, sessionID)
			if idleSnapErr == nil && strings.TrimSpace(snapshot.Last) != "" {
				lastSnapshot = snapshot
				return strings.TrimSpace(snapshot.Last), nil
			}
		}

		if waitCtx.Err() != nil {
			if errors.Is(waitCtx.Err(), context.Canceled) {
				return "", context.Canceled
			}
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				return "", fmt.Errorf("timeout waiting for OpenCode response")
			}
			return "", waitCtx.Err()
		}

		select {
		case <-waitCtx.Done():
			if errors.Is(waitCtx.Err(), context.Canceled) {
				return "", context.Canceled
			}
			return "", fmt.Errorf("timeout waiting for OpenCode response (state=%s assistant_count=%d last_len=%d)", lastState, lastSnapshot.Count, len(lastSnapshot.Last))
		case <-time.After(2 * time.Second):
		}
	}
}

func isIdleState(state string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(state))
	return trimmed == "idle" || trimmed == "completed" || trimmed == "done" || trimmed == "ready"
}

func isErrorState(state string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(state))
	return strings.Contains(trimmed, "error") || strings.Contains(trimmed, "failed") || strings.Contains(trimmed, "abort")
}

func formatSessionTimeLabel(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "--:--"
	}
	trimmed = strings.Join(strings.Fields(trimmed), " ")
	trimmed = strings.ReplaceAll(trimmed, "•", "·")
	trimmed = strings.ReplaceAll(trimmed, " ·", " · ")
	trimmed = strings.ReplaceAll(trimmed, "· ", " · ")
	trimmed = strings.Join(strings.Fields(trimmed), " ")

	if unixMs, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return time.UnixMilli(normalizeUnixMillis(unixMs)).Local().Format("15:04")
	}
	if unixFloat, err := strconv.ParseFloat(trimmed, 64); err == nil {
		return time.UnixMilli(normalizeUnixMillis(int64(unixFloat))).Local().Format("15:04")
	}
	if parsed, err := time.Parse("3:04 PM", strings.ToUpper(trimmed)); err == nil {
		return parsed.Local().Format("15:04")
	}
	if parsed, err := time.Parse("3:04 PM · 1/2/2006", strings.ToUpper(trimmed)); err == nil {
		return parsed.Local().Format("15:04")
	}
	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return parsed.Local().Format("15:04")
	}
	return "--:--"
}

func normalizeUnixMillis(raw int64) int64 {
	abs := raw
	if abs < 0 {
		abs = -abs
	}
	switch {
	case abs < 1_000_000_000_0:
		return raw * 1000
	case abs > 9_999_999_999_999_999:
		return raw / 1_000_000
	case abs > 9_999_999_999_999:
		return raw / 1000
	default:
		return raw
	}
}

func userFacingOpenCodeError(err error) string {
	if err == nil {
		return "No se pudo enviar el mensaje a OpenCode."
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	if strings.Contains(text, "connect: connection refused") || strings.Contains(text, "no such host") {
		return "OpenCode no esta disponible. Revisa OPENCODE_SERVER_URL y que el servidor este corriendo."
	}
	if strings.Contains(text, "status 401") || strings.Contains(text, "status 403") || strings.Contains(text, "unauthorized") {
		return "OpenCode rechazo credenciales. Revisa OPENCODE_SERVER_USERNAME y OPENCODE_SERVER_PASSWORD."
	}
	if strings.Contains(text, "context deadline exceeded") || strings.Contains(text, "timeout") {
		return "OpenCode no respondio a tiempo. Revisa OPENCODE_TIMEOUT_MS o la carga del servidor."
	}
	return "No se pudo enviar el mensaje a OpenCode."
}
