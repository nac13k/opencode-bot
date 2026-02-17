package opencode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/config"
)

type Client struct {
	baseURL  string
	username string
	password string
	timeout  time.Duration
	http     *http.Client
}

type Event struct {
	Type      string
	SessionID string
	Text      string
	Final     bool
}

type SessionSummary struct {
	ID      string
	Title   string
	Updated string
}

type StatusReport struct {
	SessionID string
	Status    string
	Model     string
}

type ModelInfo struct {
	ID       string
	Name     string
	Favorite bool
}

func NewClient(cfg config.Config) *Client {
	return &Client{
		baseURL:  strings.TrimRight(cfg.OpenCodeServerURL, "/"),
		username: cfg.OpenCodeServerUser,
		password: cfg.OpenCodeServerPass,
		timeout:  cfg.OpenCodeTimeout,
		http:     &http.Client{Timeout: cfg.OpenCodeTimeout},
	}
}

func CheckConnectivity(ctx context.Context, cfg config.Config) error {
	client := NewClient(cfg)
	_, err := client.request(ctx, http.MethodGet, "/global/health", nil)
	return err
}

func (c *Client) RunPrompt(ctx context.Context, prompt string, sessionID string, model string) (string, error) {
	resolved := strings.TrimSpace(sessionID)
	if resolved == "" {
		created, err := c.CreateSession(ctx)
		if err != nil {
			return "", err
		}
		resolved = created
	}

	body := map[string]any{
		"parts": []map[string]string{{
			"type": "text",
			"text": prompt,
		}},
	}
	if strings.TrimSpace(model) != "" {
		body["model"] = strings.TrimSpace(model)
	}
	if _, err := c.request(ctx, http.MethodPost, "/session/"+resolved+"/message", body); err != nil {
		if strings.Contains(err.Error(), "status 404") {
			created, createErr := c.CreateSession(ctx)
			if createErr != nil {
				return "", createErr
			}
			resolved = created
			if _, retryErr := c.request(ctx, http.MethodPost, "/session/"+resolved+"/message", body); retryErr != nil {
				return "", retryErr
			}
			return resolved, nil
		}
		return "", err
	}
	return resolved, nil
}

func (c *Client) CreateSession(ctx context.Context) (string, error) {
	raw, err := c.request(ctx, http.MethodPost, "/session", nil)
	if err != nil {
		return "", err
	}
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", err
	}
	if payload.ID == "" {
		return "", fmt.Errorf("opencode create session returned empty id")
	}
	return payload.ID, nil
}

func (c *Client) GetLastAssistantMessage(ctx context.Context, sessionID string) (string, error) {
	raw, err := c.request(ctx, http.MethodGet, "/session/"+sessionID+"/message", nil)
	if err != nil {
		return "", err
	}

	var messages []map[string]any
	if err := json.Unmarshal(raw, &messages); err != nil {
		return "", err
	}

	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		role, _ := message["role"].(string)
		if role != "assistant" {
			continue
		}
		text := extractText(message)
		if strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text), nil
		}
	}

	return "", nil
}

func (c *Client) CompactSession(ctx context.Context, sessionID string) error {
	body := map[string]any{"command": "compact", "arguments": []string{}}
	_, err := c.request(ctx, http.MethodPost, "/session/"+sessionID+"/command", body)
	return err
}

func (c *Client) GetStatus(ctx context.Context, sessionID string) (StatusReport, error) {
	if strings.TrimSpace(sessionID) == "" {
		return StatusReport{SessionID: "", Status: "unknown", Model: ""}, nil
	}

	raw, err := c.request(ctx, http.MethodGet, "/session/status", nil)
	if err != nil {
		return StatusReport{}, err
	}
	var statusMap map[string]map[string]any
	if err := json.Unmarshal(raw, &statusMap); err != nil {
		return StatusReport{}, err
	}

	status := "unknown"
	if item, ok := statusMap[sessionID]; ok {
		status = firstString(item, "type", "status", "state")
		if status == "" {
			status = "unknown"
		}
	}

	sessionRaw, sessionErr := c.request(ctx, http.MethodGet, "/session/"+sessionID, nil)
	model := ""
	if sessionErr == nil {
		var session map[string]any
		if err := json.Unmarshal(sessionRaw, &session); err == nil {
			model = sessionModel(session)
		}
	}

	return StatusReport{SessionID: sessionID, Status: status, Model: model}, nil
}

func (c *Client) ListSessionsWithCurrent(ctx context.Context, currentSessionID string, limit int) ([]SessionSummary, error) {
	sessions, err := c.listSessions(ctx)
	if err != nil {
		return nil, err
	}

	if currentSessionID != "" {
		exists := false
		for _, item := range sessions {
			if item.ID == currentSessionID {
				exists = true
				break
			}
		}
		if !exists {
			raw, err := c.request(ctx, http.MethodGet, "/session/"+currentSessionID, nil)
			if err == nil {
				var session map[string]any
				if unmarshalErr := json.Unmarshal(raw, &session); unmarshalErr == nil {
					sessions = append(sessions, sessionToSummary(session))
				}
			}
		}
	}

	sort.Slice(sessions, func(i, j int) bool {
		return parseTimestamp(sessions[i].Updated) > parseTimestamp(sessions[j].Updated)
	})
	if limit <= 0 {
		limit = 5
	}
	if len(sessions) > limit {
		sessions = sessions[:limit]
	}
	return sessions, nil
}

func (c *Client) ListFavoriteModels(ctx context.Context) ([]ModelInfo, error) {
	rawConfig, err := c.request(ctx, http.MethodGet, "/config", nil)
	if err != nil {
		return nil, err
	}

	var configPayload map[string]any
	if err := json.Unmarshal(rawConfig, &configPayload); err != nil {
		return nil, err
	}
	fromConfig := extractFavoriteModelsFromConfig(configPayload)
	if len(fromConfig) > 0 {
		return fromConfig, nil
	}

	rawProviders, err := c.request(ctx, http.MethodGet, "/config/providers", nil)
	if err != nil {
		return nil, err
	}
	var providersPayload map[string]any
	if err := json.Unmarshal(rawProviders, &providersPayload); err != nil {
		return nil, err
	}
	return extractFavoriteModelsFromProviders(providersPayload), nil
}

func (c *Client) listSessions(ctx context.Context) ([]SessionSummary, error) {
	raw, err := c.request(ctx, http.MethodGet, "/session", nil)
	if err != nil {
		return nil, err
	}
	var payload []map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	out := make([]SessionSummary, 0, len(payload))
	for _, item := range payload {
		summary := sessionToSummary(item)
		if summary.ID == "" {
			continue
		}
		out = append(out, summary)
	}
	return out, nil
}

func (c *Client) StreamEvents(ctx context.Context) (<-chan Event, <-chan error) {
	events := make(chan Event)
	errs := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errs)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/event", nil)
		if err != nil {
			errs <- err
			return
		}
		req.SetBasicAuth(c.username, c.password)
		req.Header.Set("Accept", "text/event-stream")

		res, err := c.http.Do(req)
		if err != nil {
			errs <- err
			return
		}
		defer res.Body.Close()

		if res.StatusCode >= 400 {
			errs <- fmt.Errorf("opencode event stream status %d", res.StatusCode)
			return
		}

		scanner := bufio.NewScanner(res.Body)
		buffer := make([]byte, 0, 64*1024)
		scanner.Buffer(buffer, 4*1024*1024)

		var data bytes.Buffer
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				event, ok := parseSSEData(strings.TrimSpace(data.String()))
				if ok {
					select {
					case events <- event:
					case <-ctx.Done():
						return
					}
				}
				data.Reset()
				continue
			}
			if strings.HasPrefix(line, "data:") {
				data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}

		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			errs <- err
		}
	}()

	return events, errs
}

func (c *Client) request(ctx context.Context, method string, path string, body any) ([]byte, error) {
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, payload)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(c.username, c.password)

	res, err := c.http.Do(req)
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
			msg = fmt.Sprintf("opencode status %d", res.StatusCode)
		}
		return nil, fmt.Errorf("%s (status %d)", msg, res.StatusCode)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return []byte("{}"), nil
	}
	return raw, nil
}

func parseSSEData(data string) (Event, bool) {
	if data == "" {
		return Event{}, false
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return Event{}, false
	}

	eventType := firstString(raw, "type", "event", "name")
	if eventType == "" {
		return Event{}, false
	}

	payload := raw
	if nested, ok := raw["data"].(map[string]any); ok {
		payload = nested
	}

	sessionID := firstString(payload, "sessionID", "sessionId", "session", "id")
	text := extractText(payload)

	final := false
	if value, ok := payload["final"].(bool); ok {
		final = value
	}
	if value, ok := payload["isFinal"].(bool); ok {
		final = final || value
	}
	if status := firstString(payload, "status", "state"); status == "final" || status == "completed" {
		final = true
	}

	return Event{Type: eventType, SessionID: sessionID, Text: strings.TrimSpace(text), Final: final}, true
}

func firstString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		if text, ok := value.(string); ok {
			trimmed := strings.TrimSpace(text)
			if trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func extractText(raw map[string]any) string {
	if text := firstString(raw, "text", "content", "message"); text != "" {
		return text
	}

	parts, ok := raw["parts"].([]any)
	if !ok {
		return ""
	}
	chunks := make([]string, 0, len(parts))
	for _, item := range parts {
		part, ok := item.(map[string]any)
		if !ok {
			continue
		}
		text := firstString(part, "text", "content")
		if text != "" {
			chunks = append(chunks, text)
		}
	}
	return strings.Join(chunks, "\n")
}

func sessionToSummary(raw map[string]any) SessionSummary {
	id := firstString(raw, "id")
	title := firstString(raw, "title")
	if title == "" {
		title = "(untitled)"
	}
	updated := ""
	if timeData, ok := raw["time"].(map[string]any); ok {
		updated = firstString(timeData, "updated")
	}
	return SessionSummary{ID: id, Title: title, Updated: updated}
}

func sessionModel(session map[string]any) string {
	messages, ok := session["messages"].([]any)
	if !ok {
		return ""
	}
	for i := len(messages) - 1; i >= 0; i-- {
		msg, ok := messages[i].(map[string]any)
		if !ok {
			continue
		}
		if firstString(msg, "role") != "assistant" {
			continue
		}
		provider := firstString(msg, "providerID", "providerId", "provider")
		model := firstString(msg, "modelID", "modelId", "model")
		if provider != "" && model != "" {
			return provider + "/" + model
		}
		if model != "" {
			return model
		}
	}
	return ""
}

func parseTimestamp(value string) int64 {
	if strings.TrimSpace(value) == "" {
		return 0
	}
	if unixMs, err := strconv.ParseInt(value, 10, 64); err == nil {
		return unixMs
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return 0
	}
	return t.UnixMilli()
}

func extractFavoriteModelsFromConfig(payload map[string]any) []ModelInfo {
	fromEntries := extractModelEntries(payload)
	if len(fromEntries) > 0 {
		favorites := make([]ModelInfo, 0, len(fromEntries))
		for _, item := range fromEntries {
			if item.Favorite && strings.TrimSpace(item.ID) != "" {
				favorites = append(favorites, item)
			}
		}
		return favorites
	}

	value, ok := payload["favoriteModels"]
	if !ok {
		return nil
	}
	list, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]ModelInfo, 0, len(list))
	for _, item := range list {
		text, ok := item.(string)
		if !ok || strings.TrimSpace(text) == "" {
			continue
		}
		trimmed := strings.TrimSpace(text)
		out = append(out, ModelInfo{ID: trimmed, Name: trimmed, Favorite: true})
	}
	return out
}

func extractFavoriteModelsFromProviders(payload map[string]any) []ModelInfo {
	providersRaw, ok := payload["providers"]
	if !ok {
		return nil
	}
	providers, ok := providersRaw.([]any)
	if !ok {
		return nil
	}

	favorites := make([]ModelInfo, 0)
	for _, item := range providers {
		provider, ok := item.(map[string]any)
		if !ok {
			continue
		}
		providerID := firstString(provider, "id")
		entries := extractModelEntries(provider)
		for _, model := range entries {
			if !model.Favorite || strings.TrimSpace(model.ID) == "" {
				continue
			}
			id := model.ID
			if providerID != "" && !strings.Contains(id, "/") {
				id = providerID + "/" + id
			}
			name := model.Name
			if strings.TrimSpace(name) == "" {
				name = id
			}
			favorites = append(favorites, ModelInfo{ID: id, Name: name, Favorite: true})
		}
	}

	return favorites
}

func extractModelEntries(payload map[string]any) []ModelInfo {
	modelsRaw, ok := payload["models"]
	if !ok {
		return nil
	}
	models, ok := modelsRaw.([]any)
	if !ok {
		return nil
	}
	out := make([]ModelInfo, 0, len(models))
	for _, item := range models {
		model, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id := firstString(model, "id")
		if id == "" {
			continue
		}
		name := firstString(model, "name")
		if name == "" {
			name = id
		}
		favorite := false
		if value, ok := model["favorite"].(bool); ok {
			favorite = value
		}
		out = append(out, ModelInfo{ID: id, Name: name, Favorite: favorite})
	}
	return out
}
