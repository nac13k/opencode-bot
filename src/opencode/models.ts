import type { JsonStore } from "../store/jsonStore.js";
import { isSessionModel } from "../store/schemas.js";

const now = () => new Date().toISOString();

export class SessionModelService {
  constructor(private readonly store: JsonStore) {}

  async getModel(telegramChatId: number, telegramUserId: number): Promise<string | null> {
    const models = (await this.store.read("sessionModels")).filter(isSessionModel);
    const existing = models.find(
      (entry) => entry.telegramChatId === telegramChatId && entry.telegramUserId === telegramUserId,
    );
    return existing?.model ?? null;
  }

  async setModel(telegramChatId: number, telegramUserId: number, model: string): Promise<void> {
    const models = (await this.store.read("sessionModels")).filter(isSessionModel);
    const existing = models.find(
      (entry) => entry.telegramChatId === telegramChatId && entry.telegramUserId === telegramUserId,
    );
    if (existing) {
      existing.model = model;
      existing.updatedAt = now();
    } else {
      models.push({ telegramChatId, telegramUserId, model, updatedAt: now() });
    }
    await this.store.write("sessionModels", models);
  }

  async clearModel(telegramChatId: number, telegramUserId: number): Promise<void> {
    const models = (await this.store.read("sessionModels")).filter(isSessionModel);
    const next = models.filter(
      (entry) => !(entry.telegramChatId === telegramChatId && entry.telegramUserId === telegramUserId),
    );
    await this.store.write("sessionModels", next);
  }
}
