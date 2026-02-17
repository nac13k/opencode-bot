package service

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/opencode"
	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/ports"
	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/telegram"
)

var sessionIDPattern = regexp.MustCompile(`^ses_[A-Za-z0-9]+$`)

type BridgeService struct {
	logger      *slog.Logger
	opencode    *opencode.Client
	telegramAPI ports.TelegramClient
	authzRepo   ports.AuthzRepository
	models      ports.SessionModelRepository
	sessions    *SessionLinkService
	queue       *KeyedQueue
}

func NewBridgeService(
	logger *slog.Logger,
	opencodeClient *opencode.Client,
	telegramClient ports.TelegramClient,
	authzRepo ports.AuthzRepository,
	modelRepo ports.SessionModelRepository,
	sessions *SessionLinkService,
) *BridgeService {
	return &BridgeService{
		logger:      logger,
		opencode:    opencodeClient,
		telegramAPI: telegramClient,
		authzRepo:   authzRepo,
		models:      modelRepo,
		sessions:    sessions,
		queue:       NewKeyedQueue(),
	}
}

func (s *BridgeService) HandleUpdate(ctx context.Context, update telegram.Update) {
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
		return s.telegramAPI.SendMessage(ctx, message.Chat.ID, "Procesando solicitud...")
	})
	if err != nil {
		s.logger.Error("handle prompt failed", "error", err, "chat_id", message.Chat.ID, "user_id", message.From.ID)
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "No se pudo enviar el mensaje a OpenCode.")
	}
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
	list, err := s.opencode.ListSessionsWithCurrent(ctx, current, 5)
	if err != nil {
		s.logger.Error("list sessions failed", "error", err)
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "No se pudieron listar sesiones de OpenCode.")
		return
	}
	if len(list) == 0 {
		_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, "No hay sesiones disponibles en OpenCode.")
		return
	}
	lines := make([]string, 0, len(list)+1)
	lines = append(lines, "Sesiones recientes:")
	for i, item := range list {
		suffix := ""
		if item.ID == current {
			suffix = " [actual]"
		}
		lines = append(lines, fmt.Sprintf("%d. %s (%s)%s", i+1, item.Title, item.ID, suffix))
	}
	_ = s.telegramAPI.SendMessage(ctx, message.Chat.ID, strings.Join(lines, "\n"))
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
