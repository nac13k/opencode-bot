import { createInterface } from "node:readline/promises";
import { stdin as input, stdout as output } from "node:process";

export interface InstallerAnswers {
  botToken: string;
  adminUserIds: number[];
  transport: "polling" | "webhook";
  dataDir: string;
  opencodeCommand: string;
  opencodeTimeoutMs: number;
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
    const transportRaw = (await rl.question("BOT_TRANSPORT [polling/webhook] (polling): ")).trim();
    const dataDirRaw = (await rl.question("DATA_DIR (./data): ")).trim();
    const opencodeCommandRaw = (await rl.question("OPENCODE_COMMAND (opencode): ")).trim();
    const timeoutRaw = (await rl.question("OPENCODE_TIMEOUT_MS (120000): ")).trim();

    return {
      botToken,
      adminUserIds: parseIds(adminRaw),
      transport: transportRaw === "webhook" ? "webhook" : "polling",
      dataDir: dataDirRaw || "./data",
      opencodeCommand: opencodeCommandRaw || "opencode",
      opencodeTimeoutMs: Number(timeoutRaw || "120000"),
    };
  } finally {
    rl.close();
  }
};
