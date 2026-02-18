package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

type API struct {
	botToken        string
	client          *http.Client
	pollingInterval time.Duration
}

type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message,omitempty"`
	Data    string   `json:"data"`
}

type Message struct {
	MessageID int64  `json:"message_id"`
	From      User   `json:"from"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text"`
}

type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type Chat struct {
	ID int64 `json:"id"`
}

type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

func NewAPI(botToken string, timeout time.Duration, pollingInterval time.Duration) *API {
	if pollingInterval <= 0 {
		pollingInterval = 2 * time.Second
	}
	return &API{botToken: botToken, client: &http.Client{Timeout: timeout}, pollingInterval: pollingInterval}
}

func (a *API) SendMessage(ctx context.Context, chatID int64, text string) error {
	body := map[string]any{"chat_id": chatID, "text": text}
	_, err := a.request(ctx, http.MethodPost, "sendMessage", body)
	return err
}

func (a *API) SendChatAction(ctx context.Context, chatID int64, action string) error {
	body := map[string]any{"chat_id": chatID, "action": action}
	_, err := a.request(ctx, http.MethodPost, "sendChatAction", body)
	return err
}

func (a *API) SendMessageWithInlineKeyboard(ctx context.Context, chatID int64, text string, rows [][]InlineKeyboardButton) error {
	body := map[string]any{
		"chat_id": chatID,
		"text":    text,
		"reply_markup": InlineKeyboardMarkup{
			InlineKeyboard: rows,
		},
	}
	_, err := a.request(ctx, http.MethodPost, "sendMessage", body)
	return err
}

func (a *API) AnswerCallbackQuery(ctx context.Context, callbackQueryID string, text string) error {
	body := map[string]any{
		"callback_query_id": callbackQueryID,
		"text":              text,
		"show_alert":        false,
	}
	_, err := a.request(ctx, http.MethodPost, "answerCallbackQuery", body)
	return err
}

func (a *API) PollUpdates(ctx context.Context, handler func(context.Context, Update)) error {
	var offset int64
	workers := make(chan struct{}, 8)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		updates, err := a.getUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}

		for _, update := range updates {
			current := update
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			workers <- struct{}{}
			go func(u Update) {
				defer func() { <-workers }()
				handler(ctx, u)
			}(current)
		}

		if len(updates) == 0 {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(a.pollingInterval):
			}
		}
	}
}

func (a *API) SetupWebhook(ctx context.Context, webhookURL string) error {
	body := map[string]any{"url": webhookURL}
	_, err := a.request(ctx, http.MethodPost, "setWebhook", body)
	return err
}

func (a *API) DeleteWebhook(ctx context.Context) error {
	_, err := a.request(ctx, http.MethodPost, "deleteWebhook", map[string]bool{"drop_pending_updates": false})
	return err
}

func (a *API) WebhookPath(webhookURL string) string {
	parsed, err := url.Parse(webhookURL)
	if err != nil {
		return "/telegram/webhook"
	}
	p := strings.TrimSpace(parsed.Path)
	if p == "" {
		return "/telegram/webhook"
	}
	if !strings.HasPrefix(p, "/") {
		return "/" + p
	}
	return path.Clean(p)
}

func (a *API) getUpdates(ctx context.Context, offset int64) ([]Update, error) {
	body := map[string]any{
		"offset":          offset,
		"timeout":         longPollSeconds(a.pollingInterval),
		"allowed_updates": []string{"message", "callback_query"},
	}
	raw, err := a.request(ctx, http.MethodPost, "getUpdates", body)
	if err != nil {
		return nil, err
	}

	var payload struct {
		OK     bool     `json:"ok"`
		Result []Update `json:"result"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if !payload.OK {
		return nil, fmt.Errorf("telegram getUpdates failed")
	}
	return payload.Result, nil
}

func longPollSeconds(interval time.Duration) int {
	seconds := int(interval / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	if seconds > 50 {
		seconds = 50
	}
	return seconds
}

func (a *API) ParseWebhookUpdate(body []byte) (Update, error) {
	var update Update
	err := json.Unmarshal(body, &update)
	return update, err
}

func (a *API) request(ctx context.Context, method string, endpoint string, body any) ([]byte, error) {
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = bytes.NewReader(raw)
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/%s", a.botToken, endpoint)
	req, err := http.NewRequestWithContext(ctx, method, url, payload)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	raw, readErr := io.ReadAll(res.Body)
	if readErr != nil {
		return nil, readErr
	}
	if res.StatusCode >= 400 {
		msg := strings.TrimSpace(string(raw))
		if msg == "" {
			msg = fmt.Sprintf("telegram status %d", res.StatusCode)
		}
		return nil, fmt.Errorf("%s", msg)
	}
	return raw, nil
}
