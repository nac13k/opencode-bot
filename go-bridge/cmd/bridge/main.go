package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/app"
	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/config"
	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/logging"
	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/opencode"
	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/service"
	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/storage"
	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/telegram"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "bridge: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return runServe()
	}

	if args[0] == "--migrate" {
		return runMigrate()
	}

	switch args[0] {
	case "serve":
		return runServe()
	case "resolve":
		return runResolve(args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runServe() error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}

	logger, err := logging.New(cfg)
	if err != nil {
		return err
	}

	store, err := storage.Open(cfg)
	if err != nil {
		return err
	}
	defer store.Close()

	if err := store.Migrate(context.Background()); err != nil {
		return err
	}

	if err := store.SeedFromConfig(context.Background(), cfg.AdminUserIDs, cfg.AllowedUserIDs); err != nil {
		return err
	}

	opencodeClient := opencode.NewClient(cfg)
	telegramAPI := telegram.NewAPI(cfg.BotToken, cfg.OpenCodeTimeout)
	resolver := telegram.NewResolver(cfg.BotToken, cfg.OpenCodeTimeout)
	resolveService := service.NewResolveService(resolver, store)
	sessionLinks := service.NewSessionLinkService(store, cfg.DefaultSessionID)
	bridgeService := service.NewBridgeService(logger, opencodeClient, telegramAPI, store, store, sessionLinks)
	controlService := service.NewControlService(store, sessionLinks, store, opencodeClient)
	relayService := service.NewRelayService(
		logger,
		opencodeClient,
		store,
		telegramAPI,
		cfg.RelayMode,
		cfg.RelayFallback,
		cfg.RelayFallbackDelayMs,
	)

	server := app.NewHealthServer(cfg, logger, func(ctx context.Context, usernames []string) app.ResolveResponse {
		result := resolveService.ResolveAndPersist(ctx, usernames)
		resolved := make([]app.ResolveItem, 0, len(result.Resolved))
		for _, item := range result.Resolved {
			resolved = append(resolved, app.ResolveItem{Username: item.Username, UserID: item.UserID})
		}
		unresolved := make([]app.ResolveItem, 0, len(result.Unresolved))
		for _, item := range result.Unresolved {
			unresolved = append(unresolved, app.ResolveItem{Username: item.Username, Reason: item.Reason})
		}
		return app.ResolveResponse{Resolved: resolved, Unresolved: unresolved}
	})
	server.SetControlService(controlService)
	var webhookServer *http.Server

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	errCh := make(chan error, 4)
	go func() {
		errCh <- server.ListenAndServe()
	}()
	go func() {
		if err := relayService.Run(ctx); err != nil {
			errCh <- err
		}
	}()

	if cfg.BotTransport == "polling" {
		if err := telegramAPI.DeleteWebhook(ctx); err != nil {
			logger.Warn("delete webhook failed before polling", "error", err)
		}
		go func() {
			errCh <- telegramAPI.PollUpdates(ctx, bridgeService.HandleUpdate)
		}()
	} else {
		if err := telegramAPI.SetupWebhook(ctx, cfg.WebhookURL); err != nil {
			return err
		}
		webhookPath := telegramAPI.WebhookPath(cfg.WebhookURL)
		mux := http.NewServeMux()
		mux.HandleFunc(webhookPath, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			update, err := telegramAPI.ParseWebhookUpdate(body)
			if err != nil {
				http.Error(w, "invalid update", http.StatusBadRequest)
				return
			}
			bridgeService.HandleUpdate(r.Context(), update)
			w.WriteHeader(http.StatusOK)
		})
		webhookServer = &http.Server{Addr: cfg.WebhookListenAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
		go func() {
			errCh <- webhookServer.ListenAndServe()
		}()
	}

	logger.Info("bridge serving", "health_port", cfg.HealthPort, "transport", cfg.BotTransport)

	select {
	case <-ctx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		logger.Info("shutting down bridge")
		if webhookServer != nil {
			if err := webhookServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
		}
		if err := server.Shutdown(shutdownCtx); err != nil && !app.IsServerClosed(err) {
			return err
		}
		return nil
	case err := <-errCh:
		if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, http.ErrServerClosed) || app.IsServerClosed(err) {
			return nil
		}
		return err
	}
}

func runMigrate() error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}

	store, err := storage.Open(cfg)
	if err != nil {
		return err
	}
	defer store.Close()

	if err := store.Migrate(context.Background()); err != nil {
		return err
	}

	if err := store.SeedFromConfig(context.Background(), cfg.AdminUserIDs, cfg.AllowedUserIDs); err != nil {
		return err
	}

	fmt.Println("migration complete")
	return nil
}

func runResolve(args []string) error {
	fs := flag.NewFlagSet("resolve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var usernamesValue string
	fs.StringVar(&usernamesValue, "usernames", "", "comma or space separated @usernames")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(usernamesValue) == "" {
		return errors.New("--usernames is required")
	}

	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}

	store, err := storage.Open(cfg)
	if err != nil {
		return err
	}
	defer store.Close()

	if err := store.Migrate(context.Background()); err != nil {
		return err
	}

	resolver := telegram.NewResolver(cfg.BotToken, cfg.OpenCodeTimeout)
	resolveService := service.NewResolveService(resolver, store)
	usernames := splitUsernames(usernamesValue)
	result := resolveService.ResolveAndPersist(context.Background(), usernames)

	for _, resolved := range result.Resolved {
		fmt.Printf("resolved %s -> %d\n", resolved.Username, resolved.UserID)
	}
	for _, unresolved := range result.Unresolved {
		fmt.Printf("unresolved %s: %s\n", unresolved.Username, unresolved.Reason)
	}

	if len(result.Unresolved) > 0 {
		fmt.Println("manual steps:")
		fmt.Println("1) Ask user to message the bot")
		fmt.Println("2) Use @userinfobot to get numeric ID")
		fmt.Println("3) Add ID manually to ADMIN_USER_IDS/ALLOWED_USER_IDS")
	}

	return nil
}

func splitUsernames(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t'
	})
	usernames := make([]string, 0, len(fields))
	for _, raw := range fields {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "@") {
			trimmed = "@" + trimmed
		}
		usernames = append(usernames, strings.ToLower(trimmed))
	}
	return usernames
}

func init() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
}
