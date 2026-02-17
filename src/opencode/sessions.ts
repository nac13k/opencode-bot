import type { JsonStore } from "../store/jsonStore.js";
import { isSessionLink } from "../store/schemas.js";

const now = () => new Date().toISOString();

export class SessionLinkService {
  constructor(
    private readonly store: JsonStore,
    private readonly defaultSessionId?: string,
  ) {}

  getDefaultSessionId(): string | null {
    return this.defaultSessionId ?? null;
  }

  async getSession(telegramChatId: number, telegramUserId: number): Promise<string | null> {
    const links = (await this.store.read("sessionLinks")).filter(isSessionLink);
    const existing = links.find(
      (link) => link.telegramChatId === telegramChatId && link.telegramUserId === telegramUserId,
    );
    return existing?.opencodeSessionId ?? null;
  }

  async setSession(telegramChatId: number, telegramUserId: number, opencodeSessionId: string): Promise<void> {
    const links = (await this.store.read("sessionLinks")).filter(isSessionLink);
    const existing = links.find(
      (link) => link.telegramChatId === telegramChatId && link.telegramUserId === telegramUserId,
    );
    if (existing) {
      existing.opencodeSessionId = opencodeSessionId;
      existing.updatedAt = now();
    } else {
      links.push({ telegramChatId, telegramUserId, opencodeSessionId, updatedAt: now() });
    }
    await this.store.write("sessionLinks", links);
  }

  async clearSession(telegramChatId: number, telegramUserId: number): Promise<void> {
    const links = (await this.store.read("sessionLinks")).filter(isSessionLink);
    const next = links.filter(
      (link) => !(link.telegramChatId === telegramChatId && link.telegramUserId === telegramUserId),
    );
    await this.store.write("sessionLinks", next);
  }
}
