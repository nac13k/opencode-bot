export const STORE_FILES = {
  allowedUsers: "allowed-users.json",
  admins: "admins.json",
  sessionLinks: "session-links.json",
  sessionModels: "session-models.json",
  lastMessages: "last-messages.json",
  usernameIndex: "username-index.json",
} as const;

export type StoreFileKey = keyof typeof STORE_FILES;
