import type { MiddlewareFn } from "grammy";

import type { BotContext, BotServices } from "./types.js";

export const attachServices = (services: BotServices): MiddlewareFn<BotContext> => {
  return async (ctx, next) => {
    ctx.services = services;
    await next();
  };
};
