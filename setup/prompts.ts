import { createInterface } from "node:readline/promises";
import { stdin as input, stdout as output } from "node:process";

export interface InstallerAnswers {
  botToken: string;
  adminUserIds: number[];
  allowedUserIds: number[];
  transport: "polling" | "webhook";
  dataDir: string;
  opencodeTimeoutMs: number;
  opencodeServerUrl: string;
  opencodeServerUsername: string;
  opencodeServerPassword: string;
}

const parseIds = (raw: string): number[] =>
  raw
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean)
    .map((item) => Number(item))
    .filter((item) => Number.isFinite(item));

export const askInstallerQuestions = async (): Promise<InstallerAnswers> => {
  const rl = createInterface({ input, output });
  try {
    const botToken = (await rl.question("BOT_TOKEN: ")).trim();
    const adminRaw = (await rl.question("ADMIN_USER_IDS (comma separated): ")).trim();
    const allowedRaw = (await rl.question("ALLOWED_USER_IDS (comma separated, optional): ")).trim();
    const transportRaw = (await rl.question("BOT_TRANSPORT [polling/webhook] (polling): ")).trim();
    const dataDirRaw = (await rl.question("DATA_DIR (./data): ")).trim();
    const timeoutRaw = (await rl.question("OPENCODE_TIMEOUT_MS (120000): ")).trim();
    const serverUrlRaw = (await rl.question("OPENCODE_SERVER_URL (http://127.0.0.1:4096): ")).trim();
    const serverUsernameRaw = (await rl.question("OPENCODE_SERVER_USERNAME (opencode): ")).trim();
    const serverPasswordRaw = (await rl.question("OPENCODE_SERVER_PASSWORD (optional): ")).trim();

    return {
      botToken,
      adminUserIds: parseIds(adminRaw),
      allowedUserIds: parseIds(allowedRaw),
      transport: transportRaw === "webhook" ? "webhook" : "polling",
      dataDir: dataDirRaw || "./data",
      opencodeTimeoutMs: Number(timeoutRaw || "120000"),
      opencodeServerUrl: serverUrlRaw || "http://127.0.0.1:4096",
      opencodeServerUsername: serverUsernameRaw || "opencode",
      opencodeServerPassword: serverPasswordRaw,
    };
  } finally {
    rl.close();
  }
};
