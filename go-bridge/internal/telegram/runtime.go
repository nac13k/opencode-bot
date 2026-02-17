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
	botToken string
	client   *http.Client
}

type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message,omitempty"`
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

func NewAPI(botToken string, timeout time.Duration) *API {
	return &API{botToken: botToken, client: &http.Client{Timeout: timeout}}
}

func (a *API) SendMessage(ctx context.Context, chatID int64, text string) error {
	body := map[string]any{"chat_id": chatID, "text": text}
	_, err := a.request(ctx, http.MethodPost, "sendMessage", body)
	return err
}

func (a *API) PollUpdates(ctx context.Context, handler func(context.Context, Update)) error {
	var offset int64
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
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			handler(ctx, update)
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
		"timeout":         30,
		"allowed_updates": []string{"message"},
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
