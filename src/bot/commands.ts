import { InlineKeyboard, type Bot } from "grammy";

import type { BotContext } from "./types.js";

const parseUserId = (value: string | undefined): number | null => {
  if (!value) return null;
  const parsed = Number(value.trim());
  if (!Number.isFinite(parsed)) return null;
  return parsed;
};

const requireFrom = async (ctx: BotContext): Promise<number | null> => {
  const fromId = ctx.from?.id;
  if (!fromId) {
    await ctx.reply("No se pudo identificar el usuario.");
    return null;
  }
  return fromId;
};

const requireAdmin = async (ctx: BotContext): Promise<number | null> => {
  const fromId = await requireFrom(ctx);
  if (!fromId) return null;
  if (!(await ctx.services.authz.isAdmin(fromId))) {
    await ctx.reply("Solo admins pueden usar este comando.");
    return null;
  }
  return fromId;
};

const requireAllowed = async (ctx: BotContext): Promise<number | null> => {
  const fromId = await requireFrom(ctx);
  if (!fromId) return null;
  if (!(await ctx.services.authz.isAllowed(fromId))) {
    await ctx.reply("No autorizado. Pide acceso al admin con tu userId.");
    return null;
  }
  return fromId;
};

const isSessionId = (value: string): boolean => /^ses_[A-Za-z0-9]+$/.test(value);

const sessionDataPrefix = "session:use:";
const sessionNewData = "session:new";
const modelDataPrefix = "model:set:";
const modelClearData = "model:clear";

const truncate = (value: string, max = 58): string => {
  if (value.length <= max) return value;
  return `${value.slice(0, Math.max(0, max - 1))}…`;
};

const buildSessionKeyboard = (sessions: Array<{ id: string; title: string }>): InlineKeyboard => {
  const keyboard = new InlineKeyboard();
  for (const session of sessions) {
    const label = truncate(`${session.title} (${session.id})`);
    keyboard.text(label, `${sessionDataPrefix}${session.id}`).row();
  }
  keyboard.text("Nueva sesion", sessionNewData);
  return keyboard;
};

const buildModelKeyboard = (models: Array<{ id: string; name: string }>): InlineKeyboard => {
  const keyboard = new InlineKeyboard();
  for (const model of models) {
    const label = truncate(model.name || model.id, 48);
    keyboard.text(label, `${modelDataPrefix}${model.id}`).row();
  }
  keyboard.text("Quitar modelo", modelClearData);
  return keyboard;
};

export const registerCommands = (bot: Bot<BotContext>): void => {
  bot.command("start", async (ctx) => {
    const fromId = await requireFrom(ctx);
    if (!fromId) return;
    await ctx.services.resolver.updateFromMessage(fromId, ctx.from?.username);
    const allowed = await ctx.services.authz.isAllowed(fromId);
    await ctx.reply(
      allowed
        ? "Bot listo. Puedes enviar instrucciones para OpenCode."
        : "No autorizado. Pide a un admin que te agregue por userId.",
    );
  });

  bot.command("allow", async (ctx) => {
    const adminId = await requireAdmin(ctx);
    if (!adminId) return;
    const arg = ctx.match?.trim();
    const userId = parseUserId(arg);
    if (!userId) {
      await ctx.reply("Uso: /allow <userId>");
      return;
    }
    await ctx.services.authz.addAllowedUser(userId, adminId);
    await ctx.reply(`Usuario ${userId} autorizado.`);
  });

  bot.command("deny", async (ctx) => {
    const adminId = await requireAdmin(ctx);
    if (!adminId) return;
    const arg = ctx.match?.trim();
    const userId = parseUserId(arg);
    if (!userId) {
      await ctx.reply("Uso: /deny <userId>");
      return;
    }
    await ctx.services.authz.removeAllowedUser(userId);
    await ctx.reply(`Usuario ${userId} removido.`);
  });

  bot.command("list", async (ctx) => {
    const adminId = await requireAdmin(ctx);
    if (!adminId) return;
    const [admins, allowed] = await Promise.all([
      ctx.services.authz.listAdminUsers(),
      ctx.services.authz.listAllowedUsers(),
    ]);
    await ctx.reply(`Admins: ${admins.join(", ") || "(vacío)"}\nAllowed: ${allowed.join(", ") || "(vacío)"}`);
  });

  bot.command("status", async (ctx) => {
    const adminId = await requireAdmin(ctx);
    if (!adminId) return;
    await ctx.reply("Bridge activo. Esperando mensajes.");
  });

  bot.command("resolve", async (ctx) => {
    const adminId = await requireAdmin(ctx);
    if (!adminId) return;
    const arg = ctx.match?.trim();
    if (!arg?.startsWith("@")) {
      await ctx.reply("Uso: /resolve @username");
      return;
    }
    const userId = await ctx.services.resolver.resolve(arg);
    await ctx.reply(userId ? `${arg} -> ${userId}` : `No fue posible resolver ${arg}`);
  });

  bot.command("session", async (ctx) => {
    const userId = await requireAllowed(ctx);
    if (!userId) return;
    const chatId = ctx.chat?.id;
    if (!chatId) {
      await ctx.reply("No se pudo identificar el chat.");
      return;
    }

    const args = ctx.match?.trim() ?? "";
    const [action, value] = args.split(/\s+/, 2);

    if (!action) {
      const current = await ctx.services.sessions.getSession(chatId, userId);
      await ctx.reply(
        `Sesion actual: ${current ?? "(nueva en el proximo mensaje)"}\n` +
          "Uso:\n" +
          "/session list\n" +
          "/session use <ses_...>\n" +
          "/session new",
      );
      return;
    }

    if (action === "list") {
      const sessions = await ctx.services.opencode.listSessions(5);
      if (sessions.length === 0) {
        await ctx.reply("No hay sesiones disponibles en OpenCode.");
        return;
      }
      const body = sessions
        .map((item, index) => `${index + 1}. ${item.title}\n   ${item.id}${item.updated ? ` · ${item.updated}` : ""}`)
        .join("\n");
      await ctx.reply(`Sesiones recientes (elige una):\n${body}`, {
        reply_markup: buildSessionKeyboard(sessions),
      });
      return;
    }

    if (action === "use") {
      if (!value || !isSessionId(value)) {
        await ctx.reply("Uso: /session use <ses_...>");
        return;
      }
      await ctx.services.sessions.setSession(chatId, userId, value);
      await ctx.reply(`Sesion seleccionada: ${value}`);
      return;
    }

    if (action === "new") {
      await ctx.services.sessions.clearSession(chatId, userId);
      await ctx.reply("Sesion reiniciada. El proximo mensaje creara una sesion nueva.");
      return;
    }

    await ctx.reply("Accion invalida. Usa: /session list | /session use <ses_...> | /session new");
  });

  bot.command("sessions", async (ctx) => {
    const userId = await requireAllowed(ctx);
    if (!userId) return;
    const sessions = await ctx.services.opencode.listSessions(5);
    if (sessions.length === 0) {
      await ctx.reply("No hay sesiones disponibles en OpenCode.");
      return;
    }
    const body = sessions
      .map((item, index) => `${index + 1}. ${item.title}\n   ${item.id}${item.updated ? ` · ${item.updated}` : ""}`)
      .join("\n");
    await ctx.reply(`Sesiones recientes (elige una):\n${body}`, {
      reply_markup: buildSessionKeyboard(sessions),
    });
  });

  bot.command("models", async (ctx) => {
    const userId = await requireAllowed(ctx);
    if (!userId) return;

    const chatId = ctx.chat?.id;
    if (!chatId) {
      await ctx.reply("No se pudo identificar el chat.");
      return;
    }

    const favorites = await ctx.services.opencode.listFavoriteModels();
    if (favorites.length === 0) {
      await ctx.reply("No hay modelos favoritos configurados.");
      return;
    }

    const current = await ctx.services.models.getModel(chatId, userId);
    const body = favorites
      .map((model, index) => {
        const label = model.name || model.id;
        return `${index + 1}. ${label}${model.id === current ? " (activo)" : ""}`;
      })
      .join("\n");

    await ctx.reply(`Modelos favoritos (elige uno):\n${body}`, {
      reply_markup: buildModelKeyboard(favorites),
    });
  });

  bot.callbackQuery(new RegExp(`^${sessionDataPrefix}`), async (ctx) => {
    const userId = await requireAllowed(ctx);
    if (!userId) {
      await ctx.answerCallbackQuery({ text: "No autorizado", show_alert: true });
      return;
    }

    const chatId = ctx.chat?.id;
    if (!chatId) {
      await ctx.answerCallbackQuery({ text: "Chat no disponible", show_alert: true });
      return;
    }

    const sessionId = ctx.callbackQuery.data.replace(sessionDataPrefix, "");
    if (!isSessionId(sessionId)) {
      await ctx.answerCallbackQuery({ text: "Sesion invalida", show_alert: true });
      return;
    }

    await ctx.services.sessions.setSession(chatId, userId, sessionId);
    await ctx.answerCallbackQuery({ text: "Sesion seleccionada" });
    await ctx.reply(`Sesion seleccionada: ${sessionId}`);
  });

  bot.callbackQuery(sessionNewData, async (ctx) => {
    const userId = await requireAllowed(ctx);
    if (!userId) {
      await ctx.answerCallbackQuery({ text: "No autorizado", show_alert: true });
      return;
    }

    const chatId = ctx.chat?.id;
    if (!chatId) {
      await ctx.answerCallbackQuery({ text: "Chat no disponible", show_alert: true });
      return;
    }

    await ctx.services.sessions.clearSession(chatId, userId);
    await ctx.answerCallbackQuery({ text: "Sesion limpiada" });
    await ctx.reply("Sesion reiniciada. El proximo mensaje creara una sesion nueva.");
  });

  bot.callbackQuery(new RegExp(`^${modelDataPrefix}`), async (ctx) => {
    const userId = await requireAllowed(ctx);
    if (!userId) {
      await ctx.answerCallbackQuery({ text: "No autorizado", show_alert: true });
      return;
    }

    const chatId = ctx.chat?.id;
    if (!chatId) {
      await ctx.answerCallbackQuery({ text: "Chat no disponible", show_alert: true });
      return;
    }

    const model = ctx.callbackQuery.data.replace(modelDataPrefix, "").trim();
    const favorites = await ctx.services.opencode.listFavoriteModels();
    const favoriteIds = new Set(favorites.map((item) => item.id));
    if (!favoriteIds.has(model)) {
      await ctx.answerCallbackQuery({ text: "Modelo invalido", show_alert: true });
      return;
    }

    await ctx.services.models.setModel(chatId, userId, model);
    await ctx.answerCallbackQuery({ text: "Modelo actualizado" });
    await ctx.reply(`Modelo activo: ${model}`);
  });

  bot.callbackQuery(modelClearData, async (ctx) => {
    const userId = await requireAllowed(ctx);
    if (!userId) {
      await ctx.answerCallbackQuery({ text: "No autorizado", show_alert: true });
      return;
    }

    const chatId = ctx.chat?.id;
    if (!chatId) {
      await ctx.answerCallbackQuery({ text: "Chat no disponible", show_alert: true });
      return;
    }

    await ctx.services.models.clearModel(chatId, userId);
    await ctx.answerCallbackQuery({ text: "Modelo limpiado" });
    await ctx.reply("Modelo reiniciado. Se usara el default de OpenCode.");
  });
};
