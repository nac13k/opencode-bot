import path from "node:path";

import { askInstallerQuestions } from "./prompts.js";
import { validateOpenCodeCommand, validateTelegramToken } from "./preflight.js";
import { initializeDataFiles, installGlobalPlugin, writeEnvFile } from "./writers.js";

const assertAnswers = (answers: Awaited<ReturnType<typeof askInstallerQuestions>>): void => {
  if (!answers.botToken) throw new Error("BOT_TOKEN is required");
  if (answers.adminUserIds.length === 0) {
    throw new Error("At least one ADMIN_USER_ID is required");
  }
  if (!Number.isFinite(answers.opencodeTimeoutMs) || answers.opencodeTimeoutMs < 1000) {
    throw new Error("OPENCODE_TIMEOUT_MS must be >= 1000");
  }
};

const run = async (): Promise<void> => {
  process.stdout.write("Telegram bridge installer\n\n");

  const answers = await askInstallerQuestions();
  assertAnswers(answers);

  process.stdout.write("- Validating Telegram token...\n");
  await validateTelegramToken(answers.botToken);

  process.stdout.write("- Validating OpenCode command...\n");
  await validateOpenCodeCommand(answers.opencodeCommand);

  process.stdout.write("- Writing .env...\n");
  await writeEnvFile(answers);

  process.stdout.write("- Initializing JSON data files...\n");
  await initializeDataFiles(answers);

  process.stdout.write("- Installing global OpenCode plugin...\n");
  const pluginDir = await installGlobalPlugin(answers);

  process.stdout.write("\nSetup complete\n");
  process.stdout.write(`- .env: ${path.resolve(".env")}\n`);
  process.stdout.write(`- data dir: ${path.resolve(answers.dataDir)}\n`);
  process.stdout.write(`- global plugin: ${pluginDir}\n`);
  process.stdout.write("\nNext: npm install && npm run dev\n");
};

run().catch((error) => {
  const message = error instanceof Error ? error.message : String(error);
  process.stderr.write(`Installer failed: ${message}\n`);
  process.exitCode = 1;
});
