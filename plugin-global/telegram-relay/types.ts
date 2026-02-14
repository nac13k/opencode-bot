export interface PluginConfig {
  dataDir: string;
  botToken: string;
}

export interface SessionLink {
  telegramChatId: number;
  telegramUserId: number;
  opencodeSessionId: string;
  updatedAt: string;
}
