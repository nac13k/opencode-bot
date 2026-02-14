import { readFile } from "node:fs/promises";
import path from "node:path";

import type { PluginConfig, SessionLink } from "./types.js";

const cache = new Map<string, string>();

export const setLastMessage = (sessionId: string, text: string): void => {
  if (!text.trim()) return;
  cache.set(sessionId, text.trim());
};

export const consumeLastMessage = (sessionId: string): string | null => {
  const value = cache.get(sessionId) ?? null;
  cache.delete(sessionId);
  return value;
};

export const loadPluginConfig = async (pluginDir: string): Promise<PluginConfig> => {
  const configPath = path.join(pluginDir, "config.json");
  const raw = await readFile(configPath, "utf8");
  const parsed = JSON.parse(raw) as Partial<PluginConfig>;
  if (!parsed.dataDir || !parsed.botToken) {
    throw new Error("Invalid telegram-relay config.json");
  }
  return { dataDir: parsed.dataDir, botToken: parsed.botToken };
};

export const findChatBySession = async (
  dataDir: string,
  sessionId: string,
): Promise<number | null> => {
  const raw = await readFile(path.join(dataDir, "session-links.json"), "utf8");
  const links = JSON.parse(raw) as SessionLink[];
  const link = links.find((item) => item.opencodeSessionId === sessionId);
  return link ? link.telegramChatId : null;
};
