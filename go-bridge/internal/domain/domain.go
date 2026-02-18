package domain

type User struct {
	TelegramUserID int64
	Username       string
}

type SessionLink struct {
	TelegramChatID    int64
	TelegramUserID    int64
	OpenCodeSessionID string
}

type RelayMode string

const (
	RelayModeLast  RelayMode = "last"
	RelayModeFinal RelayMode = "final"
)
