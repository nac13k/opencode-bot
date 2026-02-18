package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/config"
)

func TestImportLegacyJSON(t *testing.T) {
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir data dir: %v", err)
	}

	write := func(name string, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dataDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	write("admins.json", `[{"telegramUserId":1001}]`)
	write("allowed-users.json", `[{"telegramUserId":2002}]`)
	write("session-links.json", `[{"telegramChatId":11,"telegramUserId":22,"opencodeSessionId":"ses_abc"}]`)
	write("session-models.json", `[{"opencodeSessionId":"ses_abc","model":"provider/model"}]`)

	cfg := config.Config{
		DataDir:      dataDir,
		DatabasePath: filepath.Join(dataDir, "bridge.db"),
	}
	store, err := Open(cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	stats, err := store.ImportLegacyJSON(ctx, dataDir)
	if err != nil {
		t.Fatalf("import legacy json: %v", err)
	}

	if stats.Admins != 1 || stats.Allowed != 1 || stats.SessionLinks != 1 || stats.SessionModels != 1 {
		t.Fatalf("unexpected import stats: %+v", stats)
	}

	if ok, err := store.IsAdmin(ctx, 1001); err != nil || !ok {
		t.Fatalf("expected imported admin user: ok=%v err=%v", ok, err)
	}
	if ok, err := store.IsAllowed(ctx, 2002); err != nil || !ok {
		t.Fatalf("expected imported allowed user: ok=%v err=%v", ok, err)
	}

	sessionID, found, err := store.GetSessionLink(ctx, 11, 22)
	if err != nil {
		t.Fatalf("get session link: %v", err)
	}
	if !found || sessionID != "ses_abc" {
		t.Fatalf("unexpected session link: found=%v sessionID=%q", found, sessionID)
	}

	model, found, err := store.GetSessionModel(ctx, "ses_abc")
	if err != nil {
		t.Fatalf("get session model: %v", err)
	}
	if !found || model != "provider/model" {
		t.Fatalf("unexpected session model: found=%v model=%q", found, model)
	}
}
