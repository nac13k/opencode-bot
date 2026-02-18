package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type LegacyImportStats struct {
	Admins        int `json:"admins"`
	Allowed       int `json:"allowed"`
	SessionLinks  int `json:"sessionLinks"`
	SessionModels int `json:"sessionModels"`
}

func (s *SQLiteStore) ImportLegacyJSON(ctx context.Context, dataDir string) (LegacyImportStats, error) {
	stats := LegacyImportStats{}
	base := strings.TrimSpace(dataDir)
	if base == "" {
		return stats, nil
	}

	admins, err := readLegacyAdmins(filepath.Join(base, "admins.json"))
	if err != nil {
		return stats, err
	}
	for _, userID := range admins {
		if err := s.UpsertAdmin(ctx, userID); err != nil {
			return stats, err
		}
		stats.Admins++
	}

	allowed, err := readLegacyAllowed(filepath.Join(base, "allowed-users.json"))
	if err != nil {
		return stats, err
	}
	for _, userID := range allowed {
		if err := s.UpsertAllowed(ctx, userID); err != nil {
			return stats, err
		}
		stats.Allowed++
	}

	sessionLinks, err := readLegacySessionLinks(filepath.Join(base, "session-links.json"))
	if err != nil {
		return stats, err
	}
	for _, item := range sessionLinks {
		if err := s.UpsertSessionLink(ctx, item.ChatID, item.UserID, item.SessionID); err != nil {
			return stats, err
		}
		stats.SessionLinks++
	}

	sessionModels, err := readLegacySessionModels(filepath.Join(base, "session-models.json"))
	if err != nil {
		return stats, err
	}
	for _, item := range sessionModels {
		if err := s.UpsertSessionModel(ctx, item.SessionID, item.Model); err != nil {
			return stats, err
		}
		stats.SessionModels++
	}

	return stats, nil
}

type legacyAdminRow struct {
	TelegramUserID int64 `json:"telegramUserId"`
}

type legacyAllowedRow struct {
	TelegramUserID int64 `json:"telegramUserId"`
}

type legacySessionLinkRow struct {
	ChatID    int64  `json:"telegramChatId"`
	UserID    int64  `json:"telegramUserId"`
	SessionID string `json:"opencodeSessionId"`
}

type legacySessionModelRow struct {
	SessionID string `json:"opencodeSessionId"`
	Model     string `json:"model"`
}

func readLegacyAdmins(path string) ([]int64, error) {
	var rows []legacyAdminRow
	if err := readLegacyJSON(path, &rows); err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.TelegramUserID <= 0 {
			continue
		}
		out = append(out, row.TelegramUserID)
	}
	return out, nil
}

func readLegacyAllowed(path string) ([]int64, error) {
	var rows []legacyAllowedRow
	if err := readLegacyJSON(path, &rows); err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.TelegramUserID <= 0 {
			continue
		}
		out = append(out, row.TelegramUserID)
	}
	return out, nil
}

func readLegacySessionLinks(path string) ([]legacySessionLinkRow, error) {
	var rows []legacySessionLinkRow
	if err := readLegacyJSON(path, &rows); err != nil {
		return nil, err
	}
	out := make([]legacySessionLinkRow, 0, len(rows))
	for _, row := range rows {
		if row.ChatID == 0 || row.UserID == 0 || strings.TrimSpace(row.SessionID) == "" {
			continue
		}
		out = append(out, legacySessionLinkRow{ChatID: row.ChatID, UserID: row.UserID, SessionID: strings.TrimSpace(row.SessionID)})
	}
	return out, nil
}

func readLegacySessionModels(path string) ([]legacySessionModelRow, error) {
	var rows []legacySessionModelRow
	if err := readLegacyJSON(path, &rows); err != nil {
		return nil, err
	}
	out := make([]legacySessionModelRow, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.SessionID) == "" || strings.TrimSpace(row.Model) == "" {
			continue
		}
		out = append(out, legacySessionModelRow{SessionID: strings.TrimSpace(row.SessionID), Model: strings.TrimSpace(row.Model)})
	}
	return out, nil
}

func readLegacyJSON(path string, out any) error {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(trimmed), out); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}
