import { mkdir, readFile, rename, rm, stat, writeFile } from "node:fs/promises";
import path from "node:path";

import { STORE_FILES, type StoreFileKey } from "./files.js";
import type {
  AdminUser,
  AllowedUser,
  LastMessage,
  SessionModel,
  SessionLink,
  UsernameIndexEntry,
} from "./types.js";

type FileValueMap = {
  allowedUsers: AllowedUser[];
  admins: AdminUser[];
  sessionLinks: SessionLink[];
  sessionModels: SessionModel[];
  lastMessages: LastMessage[];
  usernameIndex: UsernameIndexEntry[];
};

const EMPTY_VALUES: FileValueMap = {
  allowedUsers: [],
  admins: [],
  sessionLinks: [],
  sessionModels: [],
  lastMessages: [],
  usernameIndex: [],
};

export class JsonStore {
  private readonly writeChains = new Map<string, Promise<void>>();

  constructor(private readonly dataDir: string) {}

  async init(): Promise<void> {
    await mkdir(this.dataDir, { recursive: true });
    await Promise.all((Object.keys(STORE_FILES) as StoreFileKey[]).map((key) => this.ensureFile(key)));
  }

  async read<K extends StoreFileKey>(key: K): Promise<FileValueMap[K]> {
    const filePath = this.resolvePath(key);
    try {
      const raw = await readFile(filePath, "utf8");
      const parsed: unknown = JSON.parse(raw);
      if (!Array.isArray(parsed)) {
        throw new Error("Expected JSON array");
      }
      return parsed as FileValueMap[K];
    } catch (error) {
      return this.tryRecover(key, error);
    }
  }

  async write<K extends StoreFileKey>(key: K, value: FileValueMap[K]): Promise<void> {
    const filePath = this.resolvePath(key);
    const chain = this.writeChains.get(filePath) ?? Promise.resolve();
    const next = chain.then(() => this.atomicWrite(filePath, value));
    this.writeChains.set(filePath, next.catch(() => undefined));
    await next;
  }

  getDataDir(): string {
    return this.dataDir;
  }

  private async ensureFile(key: StoreFileKey): Promise<void> {
    const filePath = this.resolvePath(key);
    try {
      await stat(filePath);
    } catch {
      await this.atomicWrite(filePath, EMPTY_VALUES[key]);
    }
  }

  private async tryRecover<K extends StoreFileKey>(
    key: K,
    originalError: unknown,
  ): Promise<FileValueMap[K]> {
    const filePath = this.resolvePath(key);
    const backupPath = `${filePath}.bak`;
    try {
      const backupRaw = await readFile(backupPath, "utf8");
      const backupParsed: unknown = JSON.parse(backupRaw);
      if (!Array.isArray(backupParsed)) {
        throw new Error("Backup JSON is not an array");
      }
      await this.atomicWrite(filePath, backupParsed);
      return backupParsed as FileValueMap[K];
    } catch {
      await this.atomicWrite(filePath, EMPTY_VALUES[key]);
      return EMPTY_VALUES[key] as FileValueMap[K];
    } finally {
      if (originalError instanceof Error) {
        process.stderr.write(`Failed to parse ${STORE_FILES[key]}: ${originalError.message}\n`);
      }
    }
  }

  private async atomicWrite(filePath: string, value: unknown): Promise<void> {
    const tmpPath = `${filePath}.tmp`;
    const bakPath = `${filePath}.bak`;
    const serialized = `${JSON.stringify(value, null, 2)}\n`;

    await writeFile(tmpPath, serialized, "utf8");

    try {
      await stat(filePath);
      await rename(filePath, bakPath);
    } catch {
      await rm(bakPath, { force: true });
    }

    await rename(tmpPath, filePath);
  }

  private resolvePath(key: StoreFileKey): string {
    return path.join(this.dataDir, STORE_FILES[key]);
  }
}
