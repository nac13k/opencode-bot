import { logger } from "../logger.js";

export interface OpenCodeClientOptions {
  timeoutMs: number;
  serverUrl: string;
  serverUsername: string;
  serverPassword?: string;
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

export interface OpenCodeSessionStatusReport {
  sessionId: string;
  model: string | null;
  status: "idle" | "busy" | "retry" | "unknown";
  statusMessage?: string;
  tokensUsed?: number;
  contextLimit?: number;
  contextPercent?: number;
  todos: {
    total: number;
    pending: number;
    inProgress: number;
    completed: number;
    cancelled: number;
  };
  files: {
    modified: number;
    added: number;
    deleted: number;
    totalChanged: number;
  };
  lastUpdatedAt?: number;
}

export class OpenCodeExecutionError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "OpenCodeExecutionError";
  }
}

class OpenCodeHttpError extends OpenCodeExecutionError {
  constructor(message: string, public readonly status?: number) {
    super(message);
    this.name = "OpenCodeHttpError";
  }
}

export class OpenCodeClient {
  constructor(private readonly options: OpenCodeClientOptions) {}

  async runPrompt(prompt: string, sessionId?: string, model?: string): Promise<OpenCodeRunResult> {
    const sendPrompt = async (candidateSessionId?: string): Promise<OpenCodeRunResult> => {
      const resolvedSessionId = candidateSessionId ?? (await this.createSessionId());
      const response = await this.sendMessage(resolvedSessionId, prompt, model);
      const text = this.extractTextFromParts(response?.parts ?? []);
      return {
        sessionId: resolvedSessionId,
        text,
      };
    };

    if (sessionId) {
      try {
        return await sendPrompt(sessionId);
      } catch (error) {
        if (error instanceof OpenCodeHttpError && error.status === 404) {
          logger.warn("Invalid session detected, creating a new session", { sessionId });
          return await sendPrompt();
        }
        throw error;
      }
    }

    return await sendPrompt();
  }

  async checkHealth(): Promise<void> {
    const result = await this.requestJson<{ healthy?: boolean }>("/global/health", { method: "GET" }, 5000);
    if (!result?.healthy) {
      throw new Error("OpenCode server is not healthy");
    }
  }

  async listSessions(limit = 5): Promise<OpenCodeSessionSummary[]> {
    const sessions = await this.requestJson<Array<OpenCodeSessionInfo>>("/session", { method: "GET" });
    const mapped = (sessions ?? []).map((session) => ({
      id: session.id,
      title: session.title ?? "(untitled)",
      updatedAt: this.parseTimestamp(session.time?.updated),
      updated: this.formatTimestamp(session.time?.updated),
    }));
    const ordered = mapped.sort((a, b) => b.updatedAt - a.updatedAt);
    return ordered.slice(0, Math.max(1, limit)).map(({ updatedAt: _, ...rest }) => rest);
  }

  async listSessionsWithCurrent(currentSessionId: string | null, limit = 5): Promise<OpenCodeSessionSummary[]> {
    const sessions = await this.listSessions(limit);
    if (!currentSessionId) return sessions;
    if (sessions.some((session) => session.id === currentSessionId)) return sessions;

    try {
      const current = await this.requestJson<OpenCodeSessionInfo>(
        `/session/${currentSessionId}`,
        { method: "GET" },
      );
      const entry = {
        id: current.id,
        title: current.title ?? "(untitled)",
        updated: this.formatTimestamp(current.time?.updated),
        updatedAt: this.parseTimestamp(current.time?.updated),
      };
      const merged = [entry, ...sessions.map((session) => ({
        ...session,
        updatedAt: this.parseTimestamp((session as { updated?: string }).updated),
      }))];
      const ordered = merged
        .sort((a, b) => b.updatedAt - a.updatedAt)
        .slice(0, Math.max(1, limit))
        .map(({ updatedAt: _, ...rest }) => rest);
      return ordered;
    } catch (error) {
      logger.warn("Failed to fetch current session for list", {
        sessionId: currentSessionId,
        message: error instanceof Error ? error.message : String(error),
      });
      return sessions;
    }
  }

  async compactSession(sessionId: string): Promise<void> {
    await this.requestJson(
      `/session/${sessionId}/command`,
      {
        method: "POST",
        body: {
          command: "compact",
          arguments: [],
        },
      },
    );
  }

  async listFavoriteModels(): Promise<OpenCodeModelInfo[]> {
    const config = await this.requestJson<unknown>("/config", { method: "GET" });
    const configFavorites = this.extractFavoriteModelsFromConfig(config);
    if (configFavorites.length > 0) {
      return configFavorites;
    }
    const providers = await this.requestJson<unknown>("/config/providers", { method: "GET" });
    return this.extractFavoriteModelsFromProviders(providers);
  }

  async getStatus(sessionId: string | null): Promise<OpenCodeSessionStatusReport> {
    const safeSessionId = sessionId ?? "";
    const [sessionStatus, sessionInfo, todos, files] = await Promise.all([
      this.requestJson<Record<string, { type: string; message?: string; attempt?: number }>>(
        "/session/status",
        { method: "GET" },
      ),
      safeSessionId
        ? this.requestJson<OpenCodeSessionInfo>(`/session/${safeSessionId}`, { method: "GET" })
        : null,
      safeSessionId
        ? this.requestJson<Array<OpenCodeTodo>>(`/session/${safeSessionId}/todo`, { method: "GET" })
        : [],
      this.requestJson<Array<OpenCodeFileStatus>>("/file/status", { method: "GET" }),
    ]);

    const statusEntry = safeSessionId ? sessionStatus?.[safeSessionId] : undefined;
    const status = this.mapSessionStatus(statusEntry?.type);
    const statusMessage = statusEntry?.message;
    const model = this.getSessionModel(sessionInfo);
    const contextLimit = this.getContextLimit(sessionInfo);
    const tokensUsed = this.getTokensUsed(sessionInfo);
    const contextPercent = this.getContextPercent(tokensUsed, contextLimit);

    const todoSummary = this.summarizeTodos(todos);
    const fileSummary = this.summarizeFiles(files);

    return {
      sessionId: safeSessionId || "",
      model,
      status,
      statusMessage,
      tokensUsed,
      contextLimit,
      contextPercent,
      todos: todoSummary,
      files: fileSummary,
      lastUpdatedAt: this.parseTimestamp(sessionInfo?.time?.updated),
    };
  }

  private async requestJson<T>(
    path: string,
    options: { method: string; body?: unknown },
    timeoutOverrideMs?: number,
  ): Promise<T> {
    const url = new URL(path, this.options.serverUrl);
    const headers = new Headers({ Accept: "application/json" });
    if (this.options.serverPassword) {
      const auth = Buffer.from(
        `${this.options.serverUsername}:${this.options.serverPassword}`,
        "utf8",
      ).toString("base64");
      headers.set("Authorization", `Basic ${auth}`);
    }

    let body: string | undefined;
    if (options.body !== undefined) {
      headers.set("Content-Type", "application/json");
      body = JSON.stringify(options.body);
    }

    const controller = new AbortController();
    const timeout = setTimeout(
      () => controller.abort(),
      timeoutOverrideMs ?? this.options.timeoutMs,
    );
    try {
      const response = await fetch(url, {
        method: options.method,
        headers,
        body,
        signal: controller.signal,
      });
      if (!response.ok) {
        const message = (await response.text()).trim();
        throw new OpenCodeHttpError(
          message || `OpenCode server error (${response.status})`,
          response.status,
        );
      }
      const raw = await response.text();
      if (!raw.trim()) {
        return undefined as T;
      }
      try {
        return JSON.parse(raw) as T;
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        throw new OpenCodeExecutionError(`OpenCode server invalid JSON: ${message}`);
      }
    } catch (error) {
      if (error instanceof OpenCodeHttpError) {
        throw error;
      }
      const message = error instanceof Error ? error.message : String(error);
      throw new OpenCodeExecutionError(`OpenCode server unavailable: ${message}`);
    } finally {
      clearTimeout(timeout);
    }
  }

  private async createSessionId(): Promise<string> {
    const session = await this.requestJson<OpenCodeSessionInfo>("/session", { method: "POST" });
    if (!session?.id) {
      throw new OpenCodeExecutionError("OpenCode session creation failed");
    }
    return session.id;
  }

  private async sendMessage(
    sessionId: string,
    prompt: string,
    model?: string,
  ): Promise<OpenCodeMessage> {
    return this.requestJson<OpenCodeMessage>(
      `/session/${sessionId}/message`,
      {
        method: "POST",
        body: {
          model,
          parts: [
            {
              type: "text",
              text: prompt,
            },
          ],
        },
      },
    );
  }

  private extractTextFromParts(parts: Array<OpenCodeMessagePart>): string {
    const textParts = parts
      .map((part) => {
        if (part.type === "text" && typeof part.text === "string") return part.text;
        if (part.type === "text" && typeof part.content === "string") return part.content;
        if (typeof part.text === "string") return part.text;
        return "";
      })
      .filter(Boolean);
    return textParts.join("\n").trim();
  }

  private extractFavoriteModelsFromConfig(config: unknown): OpenCodeModelInfo[] {
    const modelEntries = this.extractModelEntries(config);
    if (modelEntries.length > 0) {
      return modelEntries.filter((item) => item.id && item.favorite);
    }

    if (!config || typeof config !== "object") return [];
    const raw = config as Record<string, unknown>;
    const favoriteList = raw.favoriteModels;
    if (Array.isArray(favoriteList)) {
      return favoriteList
        .filter((item): item is string => typeof item === "string")
        .map((id) => ({ id, name: id, favorite: true }));
    }

    return [];
  }

  private extractFavoriteModelsFromProviders(providersPayload: unknown): OpenCodeModelInfo[] {
    if (!providersPayload || typeof providersPayload !== "object") return [];
    const raw = providersPayload as Record<string, unknown>;
    const providers = raw.providers;
    if (!Array.isArray(providers)) return [];

    const favorites: OpenCodeModelInfo[] = [];
    for (const provider of providers) {
      if (!provider || typeof provider !== "object") continue;
      const providerRecord = provider as Record<string, unknown>;
      const providerId = typeof providerRecord.id === "string" ? providerRecord.id : "";
      const models = providerRecord.models;
      if (!Array.isArray(models)) continue;
      const modelEntries = this.extractModelEntries({ models });
      for (const model of modelEntries) {
        if (!model.favorite || !model.id) continue;
        favorites.push({
          id: providerId && !model.id.includes("/") ? `${providerId}/${model.id}` : model.id,
          name: model.name || model.id,
          favorite: true,
        });
      }
    }
    return favorites;
  }

  private extractModelEntries(payload: unknown): Array<OpenCodeModelInfo> {
    if (!payload || typeof payload !== "object") return [];
    const raw = payload as Record<string, unknown>;
    const models = raw.models;
    if (!Array.isArray(models)) return [];
    return models
      .filter((item): item is Record<string, unknown> => !!item && typeof item === "object")
      .map((item) => {
        const idValue = item.id;
        const nameValue = item.name;
        const id = typeof idValue === "string" ? idValue : "";
        const name = typeof nameValue === "string" ? nameValue : id;
        return {
          id,
          name,
          favorite: Boolean(item.favorite),
        };
      });
  }

  private parseTimestamp(value: unknown): number {
    if (typeof value === "number" && Number.isFinite(value)) return value;
    if (typeof value === "string") {
      const parsed = Date.parse(value);
      if (Number.isFinite(parsed)) return parsed;
    }
    return 0;
  }

  private formatTimestamp(value: unknown): string | undefined {
    if (typeof value === "number" && Number.isFinite(value)) {
      return new Date(value).toISOString();
    }
    if (typeof value === "string") {
      const parsed = Date.parse(value);
      if (Number.isFinite(parsed)) return new Date(parsed).toISOString();
    }
    return undefined;
  }

  private mapSessionStatus(value?: string): "idle" | "busy" | "retry" | "unknown" {
    if (value === "idle" || value === "busy" || value === "retry") return value;
    return "unknown";
  }

  private getSessionModel(info: OpenCodeSessionInfo | null): string | null {
    if (!info?.messages?.length) return null;
    const last = info.messages
      .filter((message) => message.role === "assistant")
      .at(-1) as OpenCodeAssistantMessage | undefined;
    if (!last) return null;
    return `${last.providerID}/${last.modelID}`;
  }

  private getContextLimit(info: OpenCodeSessionInfo | null): number | undefined {
    return info?.model?.limit?.context;
  }

  private getTokensUsed(info: OpenCodeSessionInfo | null): number | undefined {
    if (!info?.messages?.length) return undefined;
    const last = info.messages
      .filter((message) => message.role === "assistant")
      .at(-1) as OpenCodeAssistantMessage | undefined;
    if (!last?.tokens) return undefined;
    const cache = last.tokens.cache?.read ?? 0;
    return last.tokens.input + last.tokens.output + last.tokens.reasoning + cache;
  }

  private getContextPercent(tokensUsed?: number, limit?: number): number | undefined {
    if (!tokensUsed || !limit) return undefined;
    return Math.min(100, Math.round((tokensUsed / limit) * 100));
  }

  private summarizeTodos(todos: Array<OpenCodeTodo>): OpenCodeSessionStatusReport["todos"] {
    const summary = {
      total: todos.length,
      pending: 0,
      inProgress: 0,
      completed: 0,
      cancelled: 0,
    };
    for (const todo of todos) {
      switch (todo.status) {
        case "pending":
          summary.pending += 1;
          break;
        case "in_progress":
          summary.inProgress += 1;
          break;
        case "completed":
          summary.completed += 1;
          break;
        case "cancelled":
          summary.cancelled += 1;
          break;
        default:
          summary.pending += 1;
      }
    }
    return summary;
  }

  private summarizeFiles(files: Array<OpenCodeFileStatus>): OpenCodeSessionStatusReport["files"] {
    const summary = {
      modified: 0,
      added: 0,
      deleted: 0,
      totalChanged: 0,
    };
    for (const file of files) {
      switch (file.status) {
        case "modified":
          summary.modified += 1;
          break;
        case "added":
          summary.added += 1;
          break;
        case "deleted":
          summary.deleted += 1;
          break;
        default:
          summary.modified += 1;
      }
      summary.totalChanged += 1;
    }
    return summary;
  }
}

type OpenCodeTodo = {
  status: "pending" | "in_progress" | "completed" | "cancelled" | string;
};

type OpenCodeFileStatus = {
  status: "added" | "deleted" | "modified" | string;
};

type OpenCodeSessionInfo = {
  id: string;
  title?: string;
  time?: { updated?: number | string };
  messages?: Array<OpenCodeAssistantMessage | OpenCodeUserMessage>;
  model?: {
    limit?: {
      context?: number;
    };
  };
};

type OpenCodeMessagePart = {
  type?: "text" | string;
  text?: string;
  content?: string;
};

type OpenCodeMessage = {
  info?: unknown;
  parts?: Array<OpenCodeMessagePart>;
};

type OpenCodeAssistantMessage = {
  role: "assistant";
  providerID: string;
  modelID: string;
  tokens: {
    input: number;
    output: number;
    reasoning: number;
    cache?: {
      read?: number;
      write?: number;
    };
  };
};

type OpenCodeUserMessage = {
  role: "user";
};
