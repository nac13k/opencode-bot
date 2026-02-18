package service

import (
	"context"
	"strings"

	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/telegram"
)

type ResolveService struct {
	resolver *telegram.Resolver
	authz    interface {
		UpsertAdmin(ctx context.Context, userID int64) error
		UpsertAllowed(ctx context.Context, userID int64) error
	}
}

func NewResolveService(resolver *telegram.Resolver, authz interface {
	UpsertAdmin(ctx context.Context, userID int64) error
	UpsertAllowed(ctx context.Context, userID int64) error
}) *ResolveService {
	return &ResolveService{resolver: resolver, authz: authz}
}

func (s *ResolveService) ResolveAndPersist(ctx context.Context, usernames []string) telegram.ResolveResult {
	normalized := normalizeUsernames(usernames)
	result := s.resolver.ResolveMany(ctx, normalized)

	for _, resolved := range result.Resolved {
		_ = s.authz.UpsertAdmin(ctx, resolved.UserID)
		_ = s.authz.UpsertAllowed(ctx, resolved.UserID)
	}

	return result
}

func normalizeUsernames(usernames []string) []string {
	out := make([]string, 0, len(usernames))
	for _, raw := range usernames {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "@") {
			trimmed = "@" + trimmed
		}
		out = append(out, strings.ToLower(trimmed))
	}
	return out
}
