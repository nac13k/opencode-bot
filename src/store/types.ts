export interface AllowedUser {
  telegramUserId: number;
  alias?: string;
  addedBy: number;
  createdAt: string;
}

export interface AdminUser {
  telegramUserId: number;
  createdAt: string;
}

export interface SessionLink {
  telegramChatId: number;
  telegramUserId: number;
  opencodeSessionId: string;
  updatedAt: string;
}

export interface LastMessage {
  opencodeSessionId: string;
  text: string;
  updatedAt: string;
}

export interface UsernameIndexEntry {
  username: string;
  telegramUserId: number;
  updatedAt: string;
}

export interface StoreState {
  allowedUsers: AllowedUser[];
  admins: AdminUser[];
  sessionLinks: SessionLink[];
  lastMessages: LastMessage[];
  usernameIndex: UsernameIndexEntry[];
}
