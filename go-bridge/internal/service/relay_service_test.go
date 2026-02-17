package service

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/opencode"
	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/ports"
)

type testRepo struct {
	recipients map[string][]ports.ChatRecipient
}

func (r *testRepo) GetSessionLink(context.Context, int64, int64) (string, bool, error) {
	return "", false, nil
}
func (r *testRepo) UpsertSessionLink(context.Context, int64, int64, string) error { return nil }
func (r *testRepo) ClearSessionLink(context.Context, int64, int64) error          { return nil }
func (r *testRepo) FindRecipientsBySession(_ context.Context, sessionID string) ([]ports.ChatRecipient, error) {
	return r.recipients[sessionID], nil
}

type sentMessage struct {
	chatID int64
	text   string
}

type testTelegram struct {
	sent []sentMessage
}

func (t *testTelegram) SendMessage(_ context.Context, chatID int64, text string) error {
	t.sent = append(t.sent, sentMessage{chatID: chatID, text: text})
	return nil
}

func TestRelayModeLastSendsCachedMessageOnIdle(t *testing.T) {
	repo := &testRepo{recipients: map[string][]ports.ChatRecipient{"ses_1": {{TelegramChatID: 10, TelegramUserID: 20}}}}
	telegramClient := &testTelegram{}
	service := NewRelayService(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		nil,
		repo,
		telegramClient,
		"last",
		true,
		1,
	)

	service.handleEvent(context.Background(), opencode.Event{Type: "message.updated", SessionID: "ses_1", Text: "hello", Final: false})
	service.handleEvent(context.Background(), opencode.Event{Type: "session.idle", SessionID: "ses_1"})

	if len(telegramClient.sent) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(telegramClient.sent))
	}
	if telegramClient.sent[0].text != "hello" {
		t.Fatalf("expected sent text hello, got %q", telegramClient.sent[0].text)
	}
}

func TestRelayModeFinalWithoutFallbackSkipsNonFinal(t *testing.T) {
	repo := &testRepo{recipients: map[string][]ports.ChatRecipient{"ses_1": {{TelegramChatID: 10, TelegramUserID: 20}}}}
	telegramClient := &testTelegram{}
	service := NewRelayService(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		nil,
		repo,
		telegramClient,
		"final",
		false,
		1,
	)

	service.handleEvent(context.Background(), opencode.Event{Type: "message.updated", SessionID: "ses_1", Text: "draft", Final: false})
	service.handleEvent(context.Background(), opencode.Event{Type: "session.idle", SessionID: "ses_1"})

	if len(telegramClient.sent) != 0 {
		t.Fatalf("expected 0 sent messages, got %d", len(telegramClient.sent))
	}
}

func TestRelayModeFinalWithFallbackSendsAfterDelay(t *testing.T) {
	repo := &testRepo{recipients: map[string][]ports.ChatRecipient{"ses_1": {{TelegramChatID: 10, TelegramUserID: 20}}}}
	telegramClient := &testTelegram{}
	service := NewRelayService(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		nil,
		repo,
		telegramClient,
		"final",
		true,
		10,
	)

	service.handleEvent(context.Background(), opencode.Event{Type: "message.updated", SessionID: "ses_1", Text: "draft", Final: false})
	service.handleEvent(context.Background(), opencode.Event{Type: "session.idle", SessionID: "ses_1"})

	if len(telegramClient.sent) != 1 {
		t.Fatalf("expected fallback to send 1 message, got %d", len(telegramClient.sent))
	}
	if telegramClient.sent[0].text != "draft" {
		t.Fatalf("expected fallback text draft, got %q", telegramClient.sent[0].text)
	}
}

func TestRelayModeFinalSendsFinalImmediately(t *testing.T) {
	repo := &testRepo{recipients: map[string][]ports.ChatRecipient{"ses_1": {{TelegramChatID: 10, TelegramUserID: 20}}}}
	telegramClient := &testTelegram{}
	service := NewRelayService(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		nil,
		repo,
		telegramClient,
		"final",
		true,
		5000,
	)

	start := time.Now()
	service.handleEvent(context.Background(), opencode.Event{Type: "message.updated", SessionID: "ses_1", Text: "final text", Final: true})
	service.handleEvent(context.Background(), opencode.Event{Type: "session.idle", SessionID: "ses_1"})
	elapsed := time.Since(start)

	if len(telegramClient.sent) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(telegramClient.sent))
	}
	if elapsed > time.Second {
		t.Fatalf("expected immediate send for final message, took %s", elapsed)
	}
}
