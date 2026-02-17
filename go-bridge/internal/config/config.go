package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	BotToken             string
	AdminUserIDs         []int64
	AllowedUserIDs       []int64
	BotTransport         string
	WebhookURL           string
	WebhookListenAddr    string
	DataDir              string
	DatabasePath         string
	OpenCodeServerURL    string
	OpenCodeServerUser   string
	OpenCodeServerPass   string
	DefaultSessionID     string
	OpenCodeTimeout      time.Duration
	RelayMode            string
	RelayFallback        bool
	RelayFallbackDelayMs int
	HealthPort           int
	LogLevel             string
	LogFilePath          string
	LogMaxSizeMB         int
	LogMaxBackups        int
	LogMaxAgeDays        int
}

func LoadFromEnv() (Config, error) {
	dataDir := defaultString(os.Getenv("DATA_DIR"), "./data")
	botToken := strings.TrimSpace(os.Getenv("BOT_TOKEN"))
	adminIDs, err := parseInt64List(os.Getenv("ADMIN_USER_IDS"))
	if err != nil {
		return Config{}, fmt.Errorf("parse ADMIN_USER_IDS: %w", err)
	}
	allowedIDs, err := parseInt64List(os.Getenv("ALLOWED_USER_IDS"))
	if err != nil {
		return Config{}, fmt.Errorf("parse ALLOWED_USER_IDS: %w", err)
	}

	openCodeTimeoutMs, err := parseIntWithDefault("OPENCODE_TIMEOUT_MS", 120000)
	if err != nil {
		return Config{}, err
	}
	healthPort, err := parseIntWithDefault("HEALTH_PORT", 4097)
	if err != nil {
		return Config{}, err
	}
	relayFallbackDelay, err := parseIntWithDefault("RELAY_FALLBACK_DELAY_MS", 3000)
	if err != nil {
		return Config{}, err
	}

	relayFallback, err := parseBoolWithDefault("RELAY_FALLBACK", true)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		BotToken:             botToken,
		AdminUserIDs:         adminIDs,
		AllowedUserIDs:       allowedIDs,
		BotTransport:         defaultString(os.Getenv("BOT_TRANSPORT"), "polling"),
		WebhookURL:           strings.TrimSpace(os.Getenv("WEBHOOK_URL")),
		WebhookListenAddr:    defaultString(strings.TrimSpace(os.Getenv("WEBHOOK_LISTEN_ADDR")), ":8090"),
		DataDir:              dataDir,
		DatabasePath:         filepath.Join(dataDir, "bridge.db"),
		OpenCodeServerURL:    defaultString(os.Getenv("OPENCODE_SERVER_URL"), "http://127.0.0.1:4096"),
		OpenCodeServerUser:   defaultString(os.Getenv("OPENCODE_SERVER_USERNAME"), "opencode"),
		OpenCodeServerPass:   strings.TrimSpace(os.Getenv("OPENCODE_SERVER_PASSWORD")),
		DefaultSessionID:     strings.TrimSpace(os.Getenv("DEFAULT_SESSION_ID")),
		OpenCodeTimeout:      time.Duration(openCodeTimeoutMs) * time.Millisecond,
		RelayMode:            defaultString(strings.TrimSpace(os.Getenv("RELAY_MODE")), "last"),
		RelayFallback:        relayFallback,
		RelayFallbackDelayMs: relayFallbackDelay,
		HealthPort:           healthPort,
		LogLevel:             defaultString(strings.TrimSpace(os.Getenv("LOG_LEVEL")), "info"),
		LogFilePath:          filepath.Join(dataDir, "logs", "bridge.log"),
		LogMaxSizeMB:         10,
		LogMaxBackups:        5,
		LogMaxAgeDays:        14,
	}

	if err := validate(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func validate(cfg Config) error {
	if cfg.BotToken == "" {
		return errors.New("BOT_TOKEN is required")
	}
	if len(cfg.AdminUserIDs) == 0 {
		return errors.New("ADMIN_USER_IDS is required")
	}
	if cfg.OpenCodeServerURL == "" {
		return errors.New("OPENCODE_SERVER_URL is required")
	}
	if cfg.BotTransport != "polling" && cfg.BotTransport != "webhook" {
		return fmt.Errorf("BOT_TRANSPORT must be polling or webhook: got %q", cfg.BotTransport)
	}
	if cfg.BotTransport == "webhook" && cfg.WebhookURL == "" {
		return errors.New("WEBHOOK_URL is required when BOT_TRANSPORT=webhook")
	}
	if cfg.BotTransport == "webhook" && strings.TrimSpace(cfg.WebhookListenAddr) == "" {
		return errors.New("WEBHOOK_LISTEN_ADDR is required when BOT_TRANSPORT=webhook")
	}
	if cfg.RelayMode != "last" && cfg.RelayMode != "final" {
		return fmt.Errorf("RELAY_MODE must be last or final: got %q", cfg.RelayMode)
	}
	if cfg.HealthPort <= 0 {
		return fmt.Errorf("HEALTH_PORT must be > 0: got %d", cfg.HealthPort)
	}
	return nil
}

func parseIntWithDefault(key string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be integer: %w", key, err)
	}
	return v, nil
}

func parseBoolWithDefault(key string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be boolean: %w", key, err)
	}
	return v, nil
}

func parseInt64List(raw string) ([]int64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	parts := strings.Split(trimmed, ",")
	out := make([]int64, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		v, err := strconv.ParseInt(item, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid numeric ID %q: %w", item, err)
		}
		out = append(out, v)
	}
	return out, nil
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
