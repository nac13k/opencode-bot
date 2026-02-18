package opencode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
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
	binary   string
	cliDir   string
	timeout  time.Duration
	http     *http.Client
	stream   *http.Client
}

var sessionIDRegex = regexp.MustCompile(`ses_[A-Za-z0-9]+`)
var sessionColumnsRegex = regexp.MustCompile(`\s{2,}`)
var cliUpdatedAtSuffixRegex = regexp.MustCompile(`(?i)\d{1,2}:\d{2}\s*(?:am|pm)(?:\s*·\s*\d{1,2}/\d{1,2}/\d{4})?$`)

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

type AssistantSnapshot struct {
	Count int
	Last  string
}

func NewClient(cfg config.Config) *Client {
	return &Client{
		baseURL:  strings.TrimRight(cfg.OpenCodeServerURL, "/"),
		username: cfg.OpenCodeServerUser,
		password: cfg.OpenCodeServerPass,
		binary:   cfg.OpenCodeBinary,
		cliDir:   cfg.OpenCodeCLIWorkDir,
		timeout:  cfg.OpenCodeTimeout,
		http:     &http.Client{Timeout: cfg.OpenCodeTimeout},
		stream:   &http.Client{},
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
	snapshot, err := c.GetAssistantSnapshot(ctx, sessionID)
	if err != nil {
		return "", err
	}
	return snapshot.Last, nil
}

func (c *Client) GetAssistantSnapshot(ctx context.Context, sessionID string) (AssistantSnapshot, error) {
	raw, err := c.request(ctx, http.MethodGet, "/session/"+sessionID+"/message", nil)
	if err != nil {
		return AssistantSnapshot{}, err
	}

	var messages []map[string]any
	if err := json.Unmarshal(raw, &messages); err != nil {
		return AssistantSnapshot{}, err
	}

	count := 0
	last := ""
	fallback := ""
	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		role, _ := message["role"].(string)
		text := extractText(message)
		if fallback == "" && strings.TrimSpace(text) != "" && !isUserRole(role) {
			fallback = strings.TrimSpace(text)
		}
		if !isAssistantRole(role) {
			continue
		}
		count++
		if last == "" && strings.TrimSpace(text) != "" {
			last = strings.TrimSpace(text)
		}
	}
	if last == "" {
		last = fallback
	}

	return AssistantSnapshot{Count: count, Last: last}, nil
}

func (c *Client) WaitForAssistantMessage(ctx context.Context, sessionID string, previous AssistantSnapshot, interval time.Duration) (string, error) {
	if interval <= 0 {
		interval = 2 * time.Second
	}

	for {
		now, err := c.GetAssistantSnapshot(ctx, sessionID)
		if err != nil {
			return "", err
		}

		if now.Count > previous.Count && strings.TrimSpace(now.Last) != "" {
			return strings.TrimSpace(now.Last), nil
		}
		if strings.TrimSpace(now.Last) != "" && strings.TrimSpace(now.Last) != strings.TrimSpace(previous.Last) {
			return strings.TrimSpace(now.Last), nil
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(interval):
		}
	}
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

func (c *Client) GetSessionState(ctx context.Context, sessionID string) (string, error) {
	if strings.TrimSpace(sessionID) == "" {
		return "unknown", nil
	}
	raw, err := c.request(ctx, http.MethodGet, "/session/status", nil)
	if err != nil {
		return "", err
	}
	var statusMap map[string]map[string]any
	if err := json.Unmarshal(raw, &statusMap); err != nil {
		return "", err
	}
	item, ok := statusMap[sessionID]
	if !ok {
		return "unknown", nil
	}
	status := firstString(item, "type", "status", "state")
	if status == "" {
		status = "unknown"
	}
	return strings.ToLower(strings.TrimSpace(status)), nil
}

func (c *Client) ListSessionsWithCurrent(ctx context.Context, currentSessionID string, limit int, source string) ([]SessionSummary, error) {
	if limit <= 0 {
		limit = 5
	}
	resolvedSource := strings.ToLower(strings.TrimSpace(source))
	if resolvedSource == "" {
		resolvedSource = "both"
	}

	sessions := make([]SessionSummary, 0)
	if resolvedSource == "endpoint" || resolvedSource == "both" {
		fromEndpoint, err := c.listSessions(ctx, maxInt(limit*4, 20))
		if err != nil && resolvedSource == "endpoint" {
			return nil, err
		}
		sessions = append(sessions, fromEndpoint...)
	}

	if resolvedSource == "cli" || resolvedSource == "both" {
		fromCLI, cliErr := c.listSessionsFromCLI(ctx)
		if cliErr != nil && resolvedSource == "cli" {
			return nil, cliErr
		}
		if cliErr == nil && len(fromCLI) > 0 {
			existing := make(map[string]struct{}, len(sessions))
			for _, item := range sessions {
				existing[item.ID] = struct{}{}
			}
			for _, cliItem := range fromCLI {
				if _, ok := existing[cliItem.ID]; ok {
					continue
				}
				sessions = append(sessions, cliItem)
				existing[cliItem.ID] = struct{}{}
			}
		}
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
	if len(sessions) > limit {
		sessions = sessions[:limit]
	}
	return sessions, nil
}

func (c *Client) listSessionsFromCLI(ctx context.Context) ([]SessionSummary, error) {
	if strings.TrimSpace(c.binary) == "" {
		return nil, fmt.Errorf("opencode binary is empty")
	}

	cmdCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, c.binary, "session", "list")
	if strings.TrimSpace(c.cliDir) != "" {
		cmd.Dir = strings.TrimSpace(c.cliDir)
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n")
	rows := make([]SessionSummary, 0)
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "Session ID") || strings.HasPrefix(line, "─") {
			continue
		}
		sessionID := firstColumn(line)
		if !sessionIDRegex.MatchString(sessionID) {
			continue
		}
		title, updated := parseCLISessionTitleAndUpdated(line, sessionID)
		rows = append(rows, SessionSummary{ID: sessionID, Title: title, Updated: updated})
	}

	if len(rows) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{}, len(rows))
	unique := make([]SessionSummary, 0, len(rows))
	for _, item := range rows {
		if item.ID == "" {
			continue
		}
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		unique = append(unique, item)
	}
	return unique, nil
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

func (c *Client) listSessions(ctx context.Context, limit int) ([]SessionSummary, error) {
	raw, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/session?limit=%d", maxInt(limit, 20)), nil)
	if err != nil {
		raw, err = c.request(ctx, http.MethodGet, "/session", nil)
	}
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

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
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

		res, err := c.stream.Do(req)
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
		updated = normalizeTimestampValue(timeData["updated"])
		if updated == "" {
			updated = normalizeTimestampValue(timeData["created"])
		}
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
	trimmed := normalizeUpdatedText(value)
	if trimmed == "" {
		return 0
	}
	if unixRaw, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return normalizeUnixMillis(unixRaw)
	}
	if parsed, err := time.Parse("3:04 PM", trimmed); err == nil {
		now := time.Now()
		withToday := time.Date(now.Year(), now.Month(), now.Day(), parsed.Hour(), parsed.Minute(), 0, 0, now.Location())
		return withToday.UnixMilli()
	}
	if parsed, err := time.Parse("3:04 PM · 1/2/2006", trimmed); err == nil {
		return parsed.UnixMilli()
	}
	if unixFloat, err := strconv.ParseFloat(trimmed, 64); err == nil {
		return normalizeUnixMillis(int64(unixFloat))
	}
	t, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return 0
	}
	return t.UnixMilli()
}

func firstColumn(line string) string {
	parts := sessionColumnsRegex.Split(strings.TrimSpace(line), 2)
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func parseCLISessionTitleAndUpdated(line string, sessionID string) (string, string) {
	remainder := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), sessionID))
	if remainder == "" {
		return "(untitled)", ""
	}

	updated := ""
	if loc := cliUpdatedAtSuffixRegex.FindStringIndex(remainder); loc != nil && loc[1] == len(remainder) {
		updated = normalizeUpdatedText(remainder[loc[0]:loc[1]])
		remainder = strings.TrimSpace(remainder[:loc[0]])
	}

	title := strings.TrimSpace(remainder)
	if title == "" {
		title = "(untitled)"
	}
	return title, updated
}

func normalizeUpdatedText(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.Join(strings.Fields(trimmed), " ")
	trimmed = strings.ReplaceAll(trimmed, "•", "·")
	trimmed = strings.ReplaceAll(trimmed, " ·", " · ")
	trimmed = strings.ReplaceAll(trimmed, "· ", " · ")
	if strings.Contains(trimmed, " · ") {
		parts := strings.SplitN(trimmed, " · ", 2)
		if len(parts) == 2 {
			trimmed = strings.ToUpper(parts[0]) + " · " + parts[1]
		}
	} else {
		trimmed = strings.ToUpper(trimmed)
	}
	return trimmed
}

func normalizeUnixMillis(raw int64) int64 {
	abs := raw
	if abs < 0 {
		abs = -abs
	}
	switch {
	case abs < 1_000_000_000_0:
		return raw * 1000
	case abs > 9_999_999_999_999_999:
		return raw / 1_000_000
	case abs > 9_999_999_999_999:
		return raw / 1000
	default:
		return raw
	}
}

func normalizeTimestampValue(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case int:
		return strconv.FormatInt(int64(v), 10)
	case json.Number:
		return v.String()
	default:
		return ""
	}
}

func isAssistantRole(role string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(role))
	if trimmed == "assistant" {
		return true
	}
	return strings.Contains(trimmed, "assistant")
}

func isUserRole(role string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(role))
	return trimmed == "user"
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
