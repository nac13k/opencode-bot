import { Bot } from "grammy";

import { attachServices } from "./middleware.js";
import { registerCommands } from "./commands.js";
import { logger } from "../logger.js";
import type { BotContext, BotServices } from "./types.js";

export const createTelegramBot = (token: string, services: BotServices): Bot<BotContext> => {
  const bot = new Bot<BotContext>(token);
  bot.use(attachServices(services));
  registerCommands(bot);

  bot.on("message:text", async (ctx) => {
    const from = ctx.from;
    const chat = ctx.chat;
    if (!from || !chat) {
      await ctx.reply("No se pudo procesar el mensaje.");
      return;
    }

    await ctx.services.resolver.updateFromMessage(from.id, from.username);

    logger.debug("Incoming telegram message", {
      chatId: chat.id,
      userId: from.id,
      username: from.username ?? null,
      textLength: ctx.message.text.length,
      isCommand: ctx.message.text.trim().startsWith("/"),
    });

    if (!(await ctx.services.authz.isAllowed(from.id))) {
      logger.warn("Unauthorized telegram message", { chatId: chat.id, userId: from.id });
      await ctx.reply("No autorizado. Pide acceso al admin con tu userId.");
      return;
    }

    const prompt = ctx.message.text.trim();
    if (!prompt || prompt.startsWith("/")) return;

    const queueKey = `${chat.id}:${from.id}`;
    await ctx.reply("Procesando en OpenCode...");
    logger.info("Queueing OpenCode prompt", {
      queueKey,
      chatId: chat.id,
      userId: from.id,
      promptPreview: prompt.slice(0, 120),
    });

    await ctx.services.queue.run(queueKey, async () => {
      try {
        const storedSessionId = await ctx.services.sessions.getSession(chat.id, from.id);
        const existingSessionId = storedSessionId?.startsWith("ses_") ? storedSessionId : null;
        if (storedSessionId && !existingSessionId) {
          logger.warn("Ignoring invalid stored OpenCode session id", {
            chatId: chat.id,
            userId: from.id,
            storedSessionId,
          });
        }
        logger.debug("Resolved session link", {
          chatId: chat.id,
          userId: from.id,
          existingSessionId,
        });
        const result = await ctx.services.opencode.runPrompt(prompt, existingSessionId ?? undefined);
        const sessionToPersist = result.sessionId ?? existingSessionId;
        if (sessionToPersist) {
          await ctx.services.sessions.setSession(chat.id, from.id, sessionToPersist);
        }

        logger.info("OpenCode response processed", {
          chatId: chat.id,
          userId: from.id,
          sessionId: sessionToPersist ?? null,
          textLength: result.text.length,
        });

        if (result.text) {
          await ctx.reply(result.text);
        } else {
          await ctx.reply("Instruccion enviada. Te respondere al terminar (idle).");
          logger.warn("OpenCode returned empty text", {
            chatId: chat.id,
            userId: from.id,
            sessionId: sessionToPersist ?? null,
          });
        }
      } catch (error) {
        const message = error instanceof Error ? error.message : "Error desconocido";
        logger.error("OpenCode execution failed", {
          chatId: chat.id,
          userId: from.id,
          message,
        });
        await ctx.reply(`Error al ejecutar OpenCode: ${message}`);
      }
    });
  });

  return bot;
};
