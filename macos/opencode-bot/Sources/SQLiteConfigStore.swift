import Foundation
import SQLite3

final class SQLiteConfigStore {
  private let dbURL: URL

  init() throws {
    let fm = FileManager.default
    let base = try fm.url(
      for: .applicationSupportDirectory,
      in: .userDomainMask,
      appropriateFor: nil,
      create: true
    )
      .appendingPathComponent("opencode-bot", isDirectory: true)

    try fm.createDirectory(at: base, withIntermediateDirectories: true)
    dbURL = base.appendingPathComponent("config.sqlite", isDirectory: false)
    try migrate()
  }

  func loadSettings() throws -> [String: String] {
    var db: OpaquePointer?
    guard sqlite3_open(dbURL.path, &db) == SQLITE_OK else {
      throw StoreError.openFailed
    }
    defer { sqlite3_close(db) }

    let query = "SELECT key, value FROM settings;"
    var stmt: OpaquePointer?
    guard sqlite3_prepare_v2(db, query, -1, &stmt, nil) == SQLITE_OK else {
      throw StoreError.prepareFailed
    }
    defer { sqlite3_finalize(stmt) }

    var output: [String: String] = [:]
    while sqlite3_step(stmt) == SQLITE_ROW {
      guard
        let keyRaw = sqlite3_column_text(stmt, 0),
        let valueRaw = sqlite3_column_text(stmt, 1)
      else {
        continue
      }
      let key = String(cString: keyRaw)
      let value = String(cString: valueRaw)
      output[key] = value
    }
    return output
  }

  func saveSettings(_ values: [String: String]) throws {
    var db: OpaquePointer?
    guard sqlite3_open(dbURL.path, &db) == SQLITE_OK else {
      throw StoreError.openFailed
    }
    defer { sqlite3_close(db) }

    guard sqlite3_exec(db, "BEGIN TRANSACTION;", nil, nil, nil) == SQLITE_OK else {
      throw StoreError.transactionFailed
    }

    let query = "INSERT INTO settings(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value;"
    var stmt: OpaquePointer?
    guard sqlite3_prepare_v2(db, query, -1, &stmt, nil) == SQLITE_OK else {
      _ = sqlite3_exec(db, "ROLLBACK;", nil, nil, nil)
      throw StoreError.prepareFailed
    }
    defer { sqlite3_finalize(stmt) }

    for (key, value) in values {
      sqlite3_reset(stmt)
      sqlite3_clear_bindings(stmt)

      sqlite3_bind_text(stmt, 1, key, -1, SQLITE_TRANSIENT)
      sqlite3_bind_text(stmt, 2, value, -1, SQLITE_TRANSIENT)

      guard sqlite3_step(stmt) == SQLITE_DONE else {
        _ = sqlite3_exec(db, "ROLLBACK;", nil, nil, nil)
        throw StoreError.writeFailed
      }
    }

    guard sqlite3_exec(db, "COMMIT;", nil, nil, nil) == SQLITE_OK else {
      _ = sqlite3_exec(db, "ROLLBACK;", nil, nil, nil)
      throw StoreError.transactionFailed
    }
  }

  private func migrate() throws {
    var db: OpaquePointer?
    guard sqlite3_open(dbURL.path, &db) == SQLITE_OK else {
      throw StoreError.openFailed
    }
    defer { sqlite3_close(db) }

    let schema = """
    CREATE TABLE IF NOT EXISTS settings (
      key TEXT PRIMARY KEY,
      value TEXT NOT NULL
    );
    """

    guard sqlite3_exec(db, schema, nil, nil, nil) == SQLITE_OK else {
      throw StoreError.migrationFailed
    }
  }

  private enum StoreError: Error {
    case openFailed
    case migrationFailed
    case prepareFailed
    case writeFailed
    case transactionFailed
  }
}

private let SQLITE_TRANSIENT = unsafeBitCast(-1, to: sqlite3_destructor_type.self)
