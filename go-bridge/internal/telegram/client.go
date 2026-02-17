package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Resolver struct {
	botToken string
	client   *http.Client
}

type ResolvedUsername struct {
	Username string
	UserID   int64
}

type UnresolvedUsername struct {
	Username string
	Reason   string
}

type ResolveResult struct {
	Resolved   []ResolvedUsername
	Unresolved []UnresolvedUsername
}

func NewResolver(botToken string, timeout time.Duration) *Resolver {
	return &Resolver{
		botToken: botToken,
		client:   &http.Client{Timeout: timeout},
	}
}

func CheckConnectivity(ctx context.Context, botToken string, timeout time.Duration) error {
	client := &http.Client{Timeout: timeout}
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", botToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}

	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode >= 400 {
		return fmt.Errorf("telegram getMe status %d", res.StatusCode)
	}
	return nil
}

func (r *Resolver) ResolveMany(ctx context.Context, usernames []string) ResolveResult {
	result := ResolveResult{
		Resolved:   make([]ResolvedUsername, 0, len(usernames)),
		Unresolved: make([]UnresolvedUsername, 0),
	}

	for _, username := range usernames {
		userID, err := r.resolveSingle(ctx, username)
		if err != nil {
			result.Unresolved = append(result.Unresolved, UnresolvedUsername{Username: username, Reason: err.Error()})
			continue
		}
		result.Resolved = append(result.Resolved, ResolvedUsername{Username: username, UserID: userID})
	}

	return result
}

func (r *Resolver) resolveSingle(ctx context.Context, username string) (int64, error) {
	cleanUsername := strings.TrimSpace(username)
	if cleanUsername == "" {
		return 0, fmt.Errorf("empty username")
	}
	if !strings.HasPrefix(cleanUsername, "@") {
		cleanUsername = "@" + cleanUsername
	}

	endpoint := fmt.Sprintf(
		"https://api.telegram.org/bot%s/getChat?chat_id=%s",
		r.botToken,
		url.QueryEscape(cleanUsername),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}

	res, err := r.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()

	var payload struct {
		OK          bool `json:"ok"`
		Description string
		Result      struct {
			ID int64 `json:"id"`
		} `json:"result"`
	}

	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return 0, err
	}
	if !payload.OK {
		reason := strings.TrimSpace(payload.Description)
		if reason == "" {
			reason = fmt.Sprintf("telegram status %d", res.StatusCode)
		}
		return 0, fmt.Errorf("%s", reason)
	}
	if payload.Result.ID == 0 {
		return 0, fmt.Errorf("telegram returned empty ID")
	}
	return payload.Result.ID, nil
}
