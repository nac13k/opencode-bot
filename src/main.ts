import { Api } from "grammy";

import { createTelegramBot } from "./bot/index.js";
import { loadConfig } from "./config.js";
import { AuthzService } from "./auth/authz.js";
import { OpenCodeClient } from "./opencode/client.js";
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
    opencodeCommand: config.opencodeCommand,
    timeoutMs: config.opencodeTimeoutMs,
  });
  const store = new JsonStore(config.dataDir);
  await store.init();

  const admins = await store.read("admins");
  if (admins.length === 0) {
    logger.info("Initializing admins from env", { count: config.admins.length });
    await store.write(
      "admins",
      config.admins.map((telegramUserId) => ({ telegramUserId, createdAt: now() })),
    );
  }

  const authz = new AuthzService(store);
  for (const adminId of config.admins) {
    await authz.addAllowedUser(adminId, adminId, "admin");
  }

  const opencode = new OpenCodeClient({
    command: config.opencodeCommand,
    timeoutMs: config.opencodeTimeoutMs,
  });
  const queue = new KeyedQueue();

  const bot = createTelegramBot(config.botToken, {
    authz,
    resolver: new UsernameResolver(store, new Api(config.botToken)),
    sessions: new SessionLinkService(store),
    opencode,
    queue,
  });

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
