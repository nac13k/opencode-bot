import type { Plugin } from "@opencode-ai/plugin";
import { fileURLToPath } from "node:url";
import path from "node:path";

import { handleEvent } from "./hooks/event.js";
import { loadPluginConfig } from "./state.js";

const pluginDir = path.dirname(fileURLToPath(import.meta.url));

export const TelegramRelayPlugin: Plugin = async () => {
  const config = await loadPluginConfig(pluginDir);
  return {
    event: handleEvent(config),
  };
};

export default TelegramRelayPlugin;
