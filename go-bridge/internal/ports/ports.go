package ports

import "context"

type ChatRecipient struct {
	TelegramChatID int64
	TelegramUserID int64
}

type AuthzRepository interface {
	IsAllowed(ctx context.Context, userID int64) (bool, error)
	IsAdmin(ctx context.Context, userID int64) (bool, error)
	UpsertAllowed(ctx context.Context, userID int64) error
	RemoveAllowed(ctx context.Context, userID int64) error
	UpsertAdmin(ctx context.Context, userID int64) error
	ListAllowed(ctx context.Context) ([]int64, error)
	ListAdmins(ctx context.Context) ([]int64, error)
}

type SessionLinkRepository interface {
	GetSessionLink(ctx context.Context, chatID int64, userID int64) (string, bool, error)
	UpsertSessionLink(ctx context.Context, chatID int64, userID int64, sessionID string) error
	ClearSessionLink(ctx context.Context, chatID int64, userID int64) error
	FindRecipientsBySession(ctx context.Context, sessionID string) ([]ChatRecipient, error)
}

type SessionModelRepository interface {
	GetSessionModel(ctx context.Context, sessionID string) (string, bool, error)
	UpsertSessionModel(ctx context.Context, sessionID string, model string) error
	ClearSessionModel(ctx context.Context, sessionID string) error
}

type TelegramClient interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
}
