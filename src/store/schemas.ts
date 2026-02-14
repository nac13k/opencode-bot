import type {
  AdminUser,
  AllowedUser,
  LastMessage,
  SessionLink,
  UsernameIndexEntry,
} from "./types.js";

const isNumber = (value: unknown): value is number => typeof value === "number" && Number.isFinite(value);
const isString = (value: unknown): value is string => typeof value === "string";

export const isAllowedUser = (value: unknown): value is AllowedUser => {
  if (!value || typeof value !== "object") return false;
  const obj = value as Partial<AllowedUser>;
  return isNumber(obj.telegramUserId) && isNumber(obj.addedBy) && isString(obj.createdAt);
};

export const isAdminUser = (value: unknown): value is AdminUser => {
  if (!value || typeof value !== "object") return false;
  const obj = value as Partial<AdminUser>;
  return isNumber(obj.telegramUserId) && isString(obj.createdAt);
};

export const isSessionLink = (value: unknown): value is SessionLink => {
  if (!value || typeof value !== "object") return false;
  const obj = value as Partial<SessionLink>;
  return (
    isNumber(obj.telegramChatId) &&
    isNumber(obj.telegramUserId) &&
    isString(obj.opencodeSessionId) &&
    isString(obj.updatedAt)
  );
};

export const isLastMessage = (value: unknown): value is LastMessage => {
  if (!value || typeof value !== "object") return false;
  const obj = value as Partial<LastMessage>;
  return isString(obj.opencodeSessionId) && isString(obj.text) && isString(obj.updatedAt);
};

export const isUsernameIndexEntry = (value: unknown): value is UsernameIndexEntry => {
  if (!value || typeof value !== "object") return false;
  const obj = value as Partial<UsernameIndexEntry>;
  return isString(obj.username) && isNumber(obj.telegramUserId) && isString(obj.updatedAt);
};
