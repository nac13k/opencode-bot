import { spawn } from "node:child_process";
import { logger } from "../logger.js";

export interface OpenCodeClientOptions {
  command: string;
  timeoutMs: number;
}

export interface OpenCodeRunResult {
  sessionId: string | null;
  text: string;
}

export interface OpenCodeSessionSummary {
  id: string;
  title: string;
  updated?: string;
}

export interface OpenCodeModelInfo {
  id: string;
  name: string;
  favorite: boolean;
}

export class OpenCodeExecutionError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "OpenCodeExecutionError";
  }
}

interface ExecResult {
  code: number | null;
  signal: NodeJS.Signals | null;
  stdout: string;
  stderr: string;
  timedOut: boolean;
}

export class OpenCodeClient {
  constructor(private readonly options: OpenCodeClientOptions) {}

  async runPrompt(prompt: string, sessionId?: string, model?: string): Promise<OpenCodeRunResult> {
    const isInvalidSessionError = (stderr: string): boolean =>
      stderr.includes("sessionID") || stderr.includes('must start with "ses"') || stderr.includes("invalid_format");

    const runOnce = async (candidateSessionId?: string) => {
      const args = ["run", "--format", "json"];
      if (model) {
        args.push("--model", model);
      }
      if (candidateSessionId) {
        args.push("--session", candidateSessionId);
      }
      args.push(prompt);
      logger.debug("Executing OpenCode command", {
        command: this.options.command,
        args: [...args.slice(0, -1), "<prompt>"] ,
        promptLength: prompt.length,
      });
      return this.exec(this.options.command, args, this.options.timeoutMs);
    };

    const first = await runOnce(sessionId);
    logger.debug("OpenCode first attempt completed", {
      code: first.code,
      signal: first.signal,
      timedOut: first.timedOut,
      stdoutLength: first.stdout.length,
      stderrLength: first.stderr.length,
      stderrPreview: first.stderr.slice(0, 240),
      usedSessionId: sessionId ?? null,
    });

    if (first.timedOut) {
      throw new Error(`OpenCode timeout after ${this.options.timeoutMs}ms`);
    }
    const firstLooksLikeInvalidSession = typeof sessionId === "string" && isInvalidSessionError(first.stderr);

    if (first.code === 0 && !firstLooksLikeInvalidSession) {
      return this.parseJsonStream(first.stdout);
    }

    const invalidSession = firstLooksLikeInvalidSession;

    if (invalidSession) {
      logger.warn("Invalid session detected, retrying without session", { sessionId });
      const retry = await runOnce();
      logger.debug("OpenCode retry completed", {
        code: retry.code,
        signal: retry.signal,
        timedOut: retry.timedOut,
        stdoutLength: retry.stdout.length,
        stderrLength: retry.stderr.length,
      });
      if (retry.timedOut) {
        throw new Error(`OpenCode timeout after ${this.options.timeoutMs}ms`);
      }
      if (retry.code === 0) {
        return this.parseJsonStream(retry.stdout);
      }
      throw new OpenCodeExecutionError(this.formatExecError(retry));
    }

    throw new OpenCodeExecutionError(this.formatExecError(first));
  }

  async checkHealth(): Promise<void> {
    const result = await this.exec(this.options.command, ["--version"], 5000);
    if (result.code !== 0) {
      throw new Error("OpenCode command is not available");
    }
  }

  async listSessions(limit = 5): Promise<OpenCodeSessionSummary[]> {
    const result = await this.exec(this.options.command, ["session", "list"], this.options.timeoutMs);
    if (result.timedOut) {
      throw new Error(`OpenCode session list timeout after ${this.options.timeoutMs}ms`);
    }
    if (result.code !== 0) {
      throw new OpenCodeExecutionError(this.formatExecError(result));
    }

    const sessions = result.stdout
      .split("\n")
      .map((line) => line.trimEnd())
      .filter((line) => line.startsWith("ses_"))
      .map((line) => {
        const idMatch = line.match(/^(ses_[A-Za-z0-9]+)/);
        const id = idMatch?.[1] ?? line;
        const rest = line.slice(id.length).trim();
        const columns = rest.split(/\s{2,}/).filter(Boolean);
        const title = columns[0] ?? "(untitled)";
        const updated = columns.slice(1).join("  ") || undefined;
        return {
          id,
          title,
          updated,
        };
      });

    return sessions.slice(0, Math.max(1, limit));
  }

  async listFavoriteModels(): Promise<OpenCodeModelInfo[]> {
    const result = await this.exec(this.options.command, ["model", "list", "--json"], this.options.timeoutMs);
    if (result.timedOut) {
      throw new Error(`OpenCode model list timeout after ${this.options.timeoutMs}ms`);
    }
    if (result.code !== 0) {
      throw new OpenCodeExecutionError(this.formatExecError(result));
    }

    try {
      const parsed = JSON.parse(result.stdout) as Array<{
        id?: string;
        name?: string;
        favorite?: boolean;
      }>;
      return parsed
        .filter((item) => item && typeof item.id === "string")
        .map((item) => ({
          id: item.id ?? item.name ?? "",
          name: item.name ?? item.id ?? "",
          favorite: Boolean(item.favorite),
        }))
        .filter((item) => item.id && item.favorite);
    } catch {
      return [];
    }
  }

  private exec(
    command: string,
    args: string[],
    timeoutMs: number,
  ): Promise<ExecResult> {
    return new Promise((resolve, reject) => {
      const child = spawn(command, args, { stdio: ["ignore", "pipe", "pipe"] });
      let stdout = "";
      let stderr = "";
      let timedOut = false;

      const timer = setTimeout(() => {
        timedOut = true;
        child.kill("SIGTERM");
        setTimeout(() => {
          if (!child.killed) {
            child.kill("SIGKILL");
          }
        }, 2000);
      }, timeoutMs);

      child.stdout.on("data", (chunk) => {
        stdout += chunk.toString();
      });
      child.stderr.on("data", (chunk) => {
        stderr += chunk.toString();
      });
      child.on("error", (error) => {
        clearTimeout(timer);
        reject(error);
      });
      child.on("close", (code, signal) => {
        clearTimeout(timer);
        resolve({ code, signal, stdout, stderr, timedOut });
      });
    });
  }

  private formatExecError(result: ExecResult): string {
    const stderr = result.stderr.trim();
    if (stderr) return stderr;
    const stdout = result.stdout.trim();
    if (stdout) return stdout;
    const exitPart = result.code === null ? `signal=${result.signal ?? "unknown"}` : `code=${result.code}`;
    return `OpenCode failed (${exitPart})`;
  }

  private parseJsonStream(output: string): OpenCodeRunResult {
    const lines = output
      .split("\n")
      .map((line) => line.trim())
      .filter(Boolean);

    let detectedSessionId: string | null = null;
    const textParts: string[] = [];

    for (const line of lines) {
      try {
        const event = JSON.parse(line) as {
          sessionID?: string;
          type?: string;
          part?: { text?: string };
        };
        if (event.sessionID && !detectedSessionId) {
          detectedSessionId = event.sessionID;
        }
        if (event.type === "text" && event.part?.text) {
          textParts.push(event.part.text);
        }
      } catch {
        continue;
      }
    }

    if (!detectedSessionId) {
      const regexMatch = output.match(/ses_[A-Za-z0-9]+/);
      detectedSessionId = regexMatch?.[0] ?? null;
    }

    const textFromJson = textParts.join("\n").trim();
    const fallbackText = lines
      .filter((line) => !line.startsWith("{"))
      .join("\n")
      .trim();

    const result = {
      sessionId: detectedSessionId,
      text: textFromJson || fallbackText,
    };
    logger.debug("Parsed OpenCode output", {
      sessionId: result.sessionId,
      textLength: result.text.length,
      lineCount: lines.length,
    });
    return result;
  }
}
