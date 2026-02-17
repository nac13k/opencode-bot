import { cp, mkdir, readFile, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

import { STORE_FILES } from "../src/store/files.js";
import type { InstallerAnswers } from "./prompts.js";

const now = () => new Date().toISOString();

export const writeEnvFile = async (answers: InstallerAnswers): Promise<void> => {
  const content = [
    `BOT_TOKEN=${answers.botToken}`,
    `ADMIN_USER_IDS=${answers.adminUserIds.join(",")}`,
    `ALLOWED_USER_IDS=${answers.allowedUserIds.join(",")}`,
    `BOT_TRANSPORT=${answers.transport}`,
    `DATA_DIR=${answers.dataDir}`,
    `OPENCODE_TIMEOUT_MS=${answers.opencodeTimeoutMs}`,
    `OPENCODE_SERVER_URL=${answers.opencodeServerUrl}`,
    `OPENCODE_SERVER_USERNAME=${answers.opencodeServerUsername}`,
    ...(answers.opencodeServerPassword
      ? [`OPENCODE_SERVER_PASSWORD=${answers.opencodeServerPassword}`]
      : []),
  ].join("\n");

  await writeFile(path.resolve(".env"), `${content}\n`, "utf8");
};

export const initializeDataFiles = async (answers: InstallerAnswers): Promise<void> => {
  const dataDir = path.resolve(answers.dataDir);
  await mkdir(dataDir, { recursive: true });

  const allowedUsers = Array.from(
    new Set([...
      answers.allowedUserIds,
      ...answers.adminUserIds,
    ]),
  );

  const payloads: Record<string, unknown> = {
    [STORE_FILES.allowedUsers]: allowedUsers.map((telegramUserId) => ({
      telegramUserId,
      alias: answers.adminUserIds.includes(telegramUserId) ? "admin" : "allowed",
      addedBy: answers.adminUserIds[0] ?? telegramUserId,
      createdAt: now(),
    })),
    [STORE_FILES.admins]: answers.adminUserIds.map((telegramUserId) => ({ telegramUserId, createdAt: now() })),
    [STORE_FILES.sessionLinks]: [],
    [STORE_FILES.sessionModels]: [],
    [STORE_FILES.lastMessages]: [],
    [STORE_FILES.usernameIndex]: [],
  };

  await Promise.all(
    Object.entries(payloads).map(([name, value]) =>
      writeFile(path.join(dataDir, name), `${JSON.stringify(value, null, 2)}\n`, "utf8"),
    ),
  );
};

export const installGlobalPlugin = async (answers: InstallerAnswers): Promise<string> => {
  const targetDir = path.join(os.homedir(), ".config", "opencode", "plugin", "telegram-relay");
  const sourceDir = path.resolve("plugin-global", "telegram-relay");

  await mkdir(targetDir, { recursive: true });
  await cp(sourceDir, targetDir, { recursive: true });

  const configPath = path.join(targetDir, "config.json");
  const config = {
    dataDir: path.resolve(answers.dataDir),
    botToken: answers.botToken,
  };
  await writeFile(configPath, `${JSON.stringify(config, null, 2)}\n`, "utf8");

  const opencodeConfigPath = path.join(os.homedir(), ".config", "opencode", "opencode.json");
  await mkdir(path.dirname(opencodeConfigPath), { recursive: true });

  let opencodeConfig: Record<string, unknown> = {};
  try {
    const raw = await readFile(opencodeConfigPath, "utf8");
    opencodeConfig = JSON.parse(raw) as Record<string, unknown>;
  } catch {
    opencodeConfig = {};
  }

  const pluginUri = `file://${path.join(targetDir, "index.ts")}`;
  const existing = Array.isArray(opencodeConfig.plugin) ? opencodeConfig.plugin : [];
  const nextPlugins = [...new Set([...existing, pluginUri])];
  opencodeConfig.plugin = nextPlugins;
  await writeFile(opencodeConfigPath, `${JSON.stringify(opencodeConfig, null, 2)}\n`, "utf8");

  return targetDir;
};
