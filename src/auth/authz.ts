import type { JsonStore } from "../store/jsonStore.js";
import { isAdminUser, isAllowedUser } from "../store/schemas.js";

const now = () => new Date().toISOString();

export class AuthzService {
  constructor(private readonly store: JsonStore) {}

  async isAllowed(userId: number): Promise<boolean> {
    const users = (await this.store.read("allowedUsers")).filter(isAllowedUser);
    return users.some((user) => user.telegramUserId === userId);
  }

  async isAdmin(userId: number): Promise<boolean> {
    const admins = (await this.store.read("admins")).filter(isAdminUser);
    return admins.some((admin) => admin.telegramUserId === userId);
  }

  async addAllowedUser(telegramUserId: number, addedBy: number, alias?: string): Promise<void> {
    const users = (await this.store.read("allowedUsers")).filter(isAllowedUser);
    if (users.some((item) => item.telegramUserId === telegramUserId)) return;
    users.push({ telegramUserId, alias, addedBy, createdAt: now() });
    await this.store.write("allowedUsers", users);
  }

  async removeAllowedUser(telegramUserId: number): Promise<void> {
    const users = (await this.store.read("allowedUsers")).filter(isAllowedUser);
    const next = users.filter((item) => item.telegramUserId !== telegramUserId);
    await this.store.write("allowedUsers", next);
  }

  async listAllowedUsers(): Promise<number[]> {
    const users = (await this.store.read("allowedUsers")).filter(isAllowedUser);
    return users.map((user) => user.telegramUserId).sort((a, b) => a - b);
  }

  async listAdminUsers(): Promise<number[]> {
    const admins = (await this.store.read("admins")).filter(isAdminUser);
    return admins.map((admin) => admin.telegramUserId).sort((a, b) => a - b);
  }
}
