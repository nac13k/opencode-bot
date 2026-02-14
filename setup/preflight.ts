import { spawn } from "node:child_process";

export const validateTelegramToken = async (token: string): Promise<void> => {
  const response = await fetch(`https://api.telegram.org/bot${token}/getMe`);
  if (!response.ok) {
    throw new Error(`Telegram validation failed: HTTP ${response.status}`);
  }
  const payload = (await response.json()) as { ok?: boolean; description?: string };
  if (!payload.ok) {
    throw new Error(`Telegram validation failed: ${payload.description ?? "unknown"}`);
  }
};

export const validateOpenCodeCommand = async (command: string): Promise<void> => {
  const code = await new Promise<number | null>((resolve, reject) => {
    const child = spawn(command, ["--version"], { stdio: "ignore" });
    child.on("error", reject);
    child.on("close", resolve);
  });
  if (code !== 0) {
    throw new Error("OpenCode command check failed");
  }
};
