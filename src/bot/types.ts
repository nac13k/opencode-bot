import type { Context } from "grammy";

import type { AuthzService } from "../auth/authz.js";
import type { OpenCodeClient } from "../opencode/client.js";
import type { SessionModelService } from "../opencode/models.js";
import type { KeyedQueue } from "../opencode/queue.js";
import type { SessionLinkService } from "../opencode/sessions.js";
import type { UsernameResolver } from "../resolver/usernameResolver.js";

export interface BotServices {
  authz: AuthzService;
  resolver: UsernameResolver;
  sessions: SessionLinkService;
  models: SessionModelService;
  opencode: OpenCodeClient;
  queue: KeyedQueue;
}

export type BotContext = Context & {
  services: BotServices;
};
