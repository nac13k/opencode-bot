package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/config"
	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/ports"
	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func Open(cfg config.Config) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.DatabasePath), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", cfg.DatabasePath)
	if err != nil {
		return nil, err
	}

	store := &SQLiteStore{db: db}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) Migrate(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS admins (
			telegram_user_id INTEGER PRIMARY KEY,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		);`,
		`CREATE TABLE IF NOT EXISTS allowed_users (
			telegram_user_id INTEGER PRIMARY KEY,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		);`,
		`CREATE TABLE IF NOT EXISTS session_links (
			telegram_chat_id INTEGER NOT NULL,
			telegram_user_id INTEGER NOT NULL,
			opencode_session_id TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT (datetime('now')),
			PRIMARY KEY (telegram_chat_id, telegram_user_id)
		);`,
		`CREATE TABLE IF NOT EXISTS session_models (
			opencode_session_id TEXT PRIMARY KEY,
			model TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		);`,
		`CREATE TABLE IF NOT EXISTS username_index (
			username TEXT PRIMARY KEY,
			telegram_user_id INTEGER NOT NULL,
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		);`,
	}

	for _, query := range queries {
		if _, err := s.db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("run migration query: %w", err)
		}
	}

	return nil
}

func (s *SQLiteStore) SeedFromConfig(ctx context.Context, adminIDs []int64, allowedIDs []int64) error {
	for _, userID := range adminIDs {
		if err := s.UpsertAdmin(ctx, userID); err != nil {
			return err
		}
	}
	for _, userID := range allowedIDs {
		if err := s.UpsertAllowed(ctx, userID); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) UpsertAdmin(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO admins (telegram_user_id, created_at)
		VALUES (?, datetime('now'))
		ON CONFLICT(telegram_user_id) DO NOTHING;
	`, userID)
	return err
}

func (s *SQLiteStore) UpsertAllowed(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO allowed_users (telegram_user_id, created_at)
		VALUES (?, datetime('now'))
		ON CONFLICT(telegram_user_id) DO NOTHING;
	`, userID)
	return err
}

func (s *SQLiteStore) IsAllowed(ctx context.Context, userID int64) (bool, error) {
	var found int
	err := s.db.QueryRowContext(ctx, `
		SELECT 1
		FROM (
			SELECT telegram_user_id FROM allowed_users
			UNION
			SELECT telegram_user_id FROM admins
		)
		WHERE telegram_user_id = ?
		LIMIT 1;
	`, userID).Scan(&found)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, err
}

func (s *SQLiteStore) IsAdmin(ctx context.Context, userID int64) (bool, error) {
	var found int
	err := s.db.QueryRowContext(ctx, `
		SELECT 1 FROM admins WHERE telegram_user_id = ? LIMIT 1;
	`, userID).Scan(&found)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, err
}

func (s *SQLiteStore) RemoveAllowed(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM allowed_users WHERE telegram_user_id = ?;`, userID)
	return err
}

func (s *SQLiteStore) ListAllowed(ctx context.Context) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT telegram_user_id FROM allowed_users;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]int64, 0)
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		out = append(out, userID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func (s *SQLiteStore) ListAdmins(ctx context.Context) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT telegram_user_id FROM admins;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]int64, 0)
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		out = append(out, userID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func (s *SQLiteStore) GetSessionLink(ctx context.Context, chatID int64, userID int64) (string, bool, error) {
	var sessionID string
	err := s.db.QueryRowContext(ctx, `
		SELECT opencode_session_id
		FROM session_links
		WHERE telegram_chat_id = ? AND telegram_user_id = ?
		LIMIT 1;
	`, chatID, userID).Scan(&sessionID)
	if err == nil {
		return sessionID, true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return "", false, err
}

func (s *SQLiteStore) UpsertSessionLink(ctx context.Context, chatID int64, userID int64, sessionID string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO session_links (telegram_chat_id, telegram_user_id, opencode_session_id, updated_at)
		VALUES (?, ?, ?, datetime('now'))
		ON CONFLICT(telegram_chat_id, telegram_user_id)
		DO UPDATE SET
			opencode_session_id = excluded.opencode_session_id,
			updated_at = datetime('now');
	`, chatID, userID, sessionID)
	return err
}

func (s *SQLiteStore) ClearSessionLink(ctx context.Context, chatID int64, userID int64) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM session_links WHERE telegram_chat_id = ? AND telegram_user_id = ?;
	`, chatID, userID)
	return err
}

func (s *SQLiteStore) FindRecipientsBySession(ctx context.Context, sessionID string) ([]ports.ChatRecipient, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT telegram_chat_id, telegram_user_id
		FROM session_links
		WHERE opencode_session_id = ?;
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ports.ChatRecipient, 0)
	for rows.Next() {
		var recipient ports.ChatRecipient
		if err := rows.Scan(&recipient.TelegramChatID, &recipient.TelegramUserID); err != nil {
			return nil, err
		}
		out = append(out, recipient)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *SQLiteStore) GetSessionModel(ctx context.Context, sessionID string) (string, bool, error) {
	var model string
	err := s.db.QueryRowContext(ctx, `
		SELECT model FROM session_models WHERE opencode_session_id = ? LIMIT 1;
	`, sessionID).Scan(&model)
	if err == nil {
		return model, true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return "", false, err
}

func (s *SQLiteStore) UpsertSessionModel(ctx context.Context, sessionID string, model string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO session_models (opencode_session_id, model, updated_at)
		VALUES (?, ?, datetime('now'))
		ON CONFLICT(opencode_session_id)
		DO UPDATE SET
			model = excluded.model,
			updated_at = datetime('now');
	`, sessionID, model)
	return err
}

func (s *SQLiteStore) ClearSessionModel(ctx context.Context, sessionID string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM session_models WHERE opencode_session_id = ?;
	`, sessionID)
	return err
}
