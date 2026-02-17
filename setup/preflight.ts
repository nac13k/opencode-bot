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

export const validateOpenCodeServer = async (
  url: string,
  username: string,
  password?: string,
): Promise<void> => {
  const headers = new Headers({ Accept: "application/json" });
  if (password) {
    const auth = Buffer.from(`${username}:${password}`, "utf8").toString("base64");
    headers.set("Authorization", `Basic ${auth}`);
  }

  const response = await fetch(new URL("/global/health", url), { headers });
  if (!response.ok) {
    throw new Error(`OpenCode server validation failed: HTTP ${response.status}`);
  }
  const payload = (await response.json()) as { healthy?: boolean };
  if (!payload.healthy) {
    throw new Error("OpenCode server validation failed: unhealthy");
  }
};
