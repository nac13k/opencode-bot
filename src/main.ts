import { Api } from "grammy";

import { createTelegramBot } from "./bot/index.js";
import { botCommandList } from "./bot/commands.js";
import { loadConfig } from "./config.js";
import { AuthzService } from "./auth/authz.js";
import { OpenCodeClient } from "./opencode/client.js";
import { SessionModelService } from "./opencode/models.js";
import { KeyedQueue } from "./opencode/queue.js";
import { SessionLinkService } from "./opencode/sessions.js";
import { UsernameResolver } from "./resolver/usernameResolver.js";
import { JsonStore } from "./store/jsonStore.js";
import { logger } from "./logger.js";

const now = () => new Date().toISOString();

const bootstrap = async (): Promise<void> => {
  const config = loadConfig();
  logger.info("Bootstrapping telegram bridge", {
    dataDir: config.dataDir,
    transport: config.transport,
    timeoutMs: config.opencodeTimeoutMs,
  });
  const store = new JsonStore(config.dataDir);
  await store.init();

  logger.info("Syncing admins from env", { count: config.admins.length });
  await store.write(
    "admins",
    config.admins.map((telegramUserId) => ({ telegramUserId, createdAt: now() })),
  );

  logger.info("Syncing allowed users from env", { count: config.allowedUsers.length });
  await store.write(
    "allowedUsers",
    config.allowedUsers.map((telegramUserId) => ({
      telegramUserId,
      alias: config.admins.includes(telegramUserId) ? "admin" : "allowed",
      addedBy: config.admins[0],
      createdAt: now(),
    })),
  );

  if (config.defaultSessionId) {
    logger.info("Default session configured", { sessionId: config.defaultSessionId });
  }

  const authz = new AuthzService(store);

  const opencode = new OpenCodeClient({
    timeoutMs: config.opencodeTimeoutMs,
    serverUrl: config.opencodeServerUrl,
    serverUsername: config.opencodeServerUsername,
    serverPassword: config.opencodeServerPassword,
  });
  const queue = new KeyedQueue();

  const bot = createTelegramBot(config.botToken, {
    authz,
    resolver: new UsernameResolver(store, new Api(config.botToken)),
    sessions: new SessionLinkService(store, config.defaultSessionId),
    models: new SessionModelService(store),
    opencode,
    queue,
  });

  await bot.api.setMyCommands(botCommandList);

  if (config.transport === "polling") {
    await bot.start();
    logger.info("Bot started", { mode: "polling" });
  } else {
    throw new Error("Webhook mode is not implemented in this MVP");
  }
};

bootstrap().catch((error) => {
  const message = error instanceof Error ? error.stack ?? error.message : String(error);
  logger.error("Bootstrap failed", { message });
  process.exitCode = 1;
});
