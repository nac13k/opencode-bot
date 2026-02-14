import type { Api } from "grammy";

import type { JsonStore } from "../store/jsonStore.js";
import { isUsernameIndexEntry } from "../store/schemas.js";

const now = () => new Date().toISOString();

const normalizeUsername = (value: string) => value.replace(/^@/, "").toLowerCase();

export class UsernameResolver {
  constructor(private readonly store: JsonStore, private readonly api: Api) {}

  async updateFromMessage(userId: number, username?: string): Promise<void> {
    if (!username) return;
    const normalized = normalizeUsername(username);
    const entries = (await this.store.read("usernameIndex")).filter(isUsernameIndexEntry);
    const existing = entries.find((entry) => entry.username === normalized);
    if (existing) {
      existing.telegramUserId = userId;
      existing.updatedAt = now();
    } else {
      entries.push({ username: normalized, telegramUserId: userId, updatedAt: now() });
    }
    await this.store.write("usernameIndex", entries);
  }

  async resolve(input: string): Promise<number | null> {
    const normalized = normalizeUsername(input);

    const entries = (await this.store.read("usernameIndex")).filter(isUsernameIndexEntry);
    const cached = entries.find((entry) => entry.username === normalized);
    if (cached) return cached.telegramUserId;

    try {
      const chat = await this.api.getChat(`@${normalized}`);
      const userId = "id" in chat ? Number(chat.id) : NaN;
      if (!Number.isFinite(userId)) return null;
      entries.push({ username: normalized, telegramUserId: userId, updatedAt: now() });
      await this.store.write("usernameIndex", entries);
      return userId;
    } catch {
      return null;
    }
  }
}
