import { consumeLastMessage, findChatBySession, setLastMessage } from "../state.js";
import { sendTelegramReply } from "../telegram.js";
import type { PluginConfig } from "../types.js";

type RelayEvent = {
  type: string;
  properties: Record<string, unknown>;
};

const extractAssistantText = (event: RelayEvent): string | null => {
  if (event.type !== "message.updated") return null;
  const info = event.properties.info as { parts?: Array<{ text?: string }> } | undefined;
  const parts = info?.parts;
  if (!parts) return null;
  const text = parts.map((part) => part.text ?? "").join("\n").trim();
  return text || null;
};

export const handleEvent =
  (config: PluginConfig) =>
  async ({ event }: { event: RelayEvent }): Promise<void> => {
    if (event.type === "message.updated") {
      const text = extractAssistantText(event);
      const info = event.properties.info as { sessionID?: string } | undefined;
      const sessionId = info?.sessionID;
      if (text && sessionId) {
        setLastMessage(sessionId, text);
      }
      return;
    }

    if (event.type === "session.idle") {
      const rawSessionId = event.properties.sessionID;
      if (typeof rawSessionId !== "string") return;
      const sessionId = rawSessionId;
      const text = consumeLastMessage(sessionId);
      if (!text) return;
      const chatId = await findChatBySession(config.dataDir, sessionId);
      if (!chatId) return;
      await sendTelegramReply(config.botToken, chatId, text);
    }
  };
