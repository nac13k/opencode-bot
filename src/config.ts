import path from "node:path";

import dotenv from "dotenv";

dotenv.config();

const env = process.env;

const required = (key: string): string => {
  const value = env[key];
  if (!value) throw new Error(`Missing required env var: ${key}`);
  return value;
};

const parseIdList = (value: string): number[] =>
  value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean)
    .map((item) => Number(item))
    .filter((item) => Number.isFinite(item));

const parseOptionalIdList = (value: string | undefined): number[] =>
  value ? parseIdList(value) : [];

export interface AppConfig {
  botToken: string;
  admins: number[];
  allowedUsers: number[];
  dataDir: string;
  opencodeCommand: string;
  opencodeTimeoutMs: number;
  opencodeServerUrl: string;
  opencodeServerUsername: string;
  opencodeServerPassword?: string;
  transport: "polling" | "webhook";
}

export const loadConfig = (): AppConfig => {
  const botToken = required("BOT_TOKEN");
  const admins = parseIdList(required("ADMIN_USER_IDS"));
  if (admins.length === 0) {
    throw new Error("ADMIN_USER_IDS must contain at least one numeric user id");
  }

  const allowedUsersRaw = parseOptionalIdList(env.ALLOWED_USER_IDS);
  const allowedUsers = [...new Set([...allowedUsersRaw, ...admins])];

  const transport = (env.BOT_TRANSPORT ?? "polling") as "polling" | "webhook";
  if (transport !== "polling" && transport !== "webhook") {
    throw new Error("BOT_TRANSPORT must be polling or webhook");
  }

  return {
    botToken,
    admins,
    allowedUsers,
    dataDir: path.resolve(env.DATA_DIR ?? "./data"),
    opencodeCommand: env.OPENCODE_COMMAND ?? "opencode",
    opencodeTimeoutMs: Number(env.OPENCODE_TIMEOUT_MS ?? "120000"),
    opencodeServerUrl: env.OPENCODE_SERVER_URL ?? "http://127.0.0.1:4096",
    opencodeServerUsername: env.OPENCODE_SERVER_USERNAME ?? "opencode",
    opencodeServerPassword: env.OPENCODE_SERVER_PASSWORD,
    transport,
  };
};
