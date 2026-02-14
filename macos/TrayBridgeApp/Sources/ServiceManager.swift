import Foundation

enum BotTransport: String, CaseIterable, Identifiable {
  case polling
  case webhook

  var id: String { rawValue }
}

final class ServiceManager: ObservableObject {
  @Published var isRunning = false
  @Published var statusText = "Ready"
  @Published var logs = ""

  @Published var botToken = ""
  @Published var adminUserIds = ""
  @Published var botTransport: BotTransport = .polling
  @Published var dataDir = "./data"
  @Published var opencodeCommand = "opencode"
  @Published var opencodeTimeoutMs = 120000

  @Published var projectPath: String
  @Published var showAdvanced = false

  private var process: Process?
  private var store: SQLiteConfigStore? = nil

  init() {
    let root = ServiceManager.detectProjectRoot()
    projectPath = root

    var loadedStore: SQLiteConfigStore?
    do {
      let candidate = try SQLiteConfigStore()
      loadedStore = candidate
      let values = try candidate.loadSettings()
      botToken = values["BOT_TOKEN"] ?? ""
      adminUserIds = values["ADMIN_USER_IDS"] ?? ""
      botTransport = BotTransport(rawValue: values["BOT_TRANSPORT"] ?? "polling") ?? .polling
      dataDir = values["DATA_DIR"] ?? "./data"
      opencodeCommand = values["OPENCODE_COMMAND"] ?? "opencode"
      opencodeTimeoutMs = Int(values["OPENCODE_TIMEOUT_MS"] ?? "120000") ?? 120000
      projectPath = values["PROJECT_PATH"] ?? projectPath
    } catch {
      statusText = "SQLite config unavailable"
      appendLog("[error] Failed to load SQLite config: \(error.localizedDescription)")
    }
    store = loadedStore
  }

  func saveConfig() {
    guard let store else {
      statusText = "SQLite config unavailable"
      appendLog("[error] Save failed: SQLite store not ready")
      return
    }
    do {
      try store.saveSettings(currentConfigValues())
      statusText = "Configuration saved"
      appendLog("[info] Saved configuration to SQLite")
    } catch {
      statusText = "Failed to save configuration"
      appendLog("[error] Save failed: \(error.localizedDescription)")
    }
  }

  func startService() {
    if isRunning {
      statusText = "Service is already running"
      return
    }

    let normalizedPath = (projectPath as NSString).expandingTildeInPath
    var isDirectory: ObjCBool = false
    if !FileManager.default.fileExists(atPath: normalizedPath, isDirectory: &isDirectory) || !isDirectory.boolValue {
      statusText = "Invalid project path"
      appendLog("[error] Invalid project path: \(normalizedPath)")
      return
    }

    if botToken.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
      statusText = "BOT_TOKEN is required"
      appendLog("[error] BOT_TOKEN is required")
      return
    }

    if adminUserIds.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
      statusText = "ADMIN_USER_IDS is required"
      appendLog("[error] ADMIN_USER_IDS is required")
      return
    }

    saveConfig()

    let proc = Process()
    proc.currentDirectoryURL = URL(fileURLWithPath: normalizedPath)
    proc.executableURL = URL(fileURLWithPath: "/bin/zsh")
    proc.arguments = ["-lc", "exec npm run dev"]

    var env = ProcessInfo.processInfo.environment
    env["BOT_TOKEN"] = botToken
    env["ADMIN_USER_IDS"] = adminUserIds
    env["BOT_TRANSPORT"] = botTransport.rawValue
    env["DATA_DIR"] = dataDir
    env["OPENCODE_COMMAND"] = opencodeCommand
    env["OPENCODE_TIMEOUT_MS"] = String(opencodeTimeoutMs)
    proc.environment = env

    let outputPipe = Pipe()
    proc.standardOutput = outputPipe
    proc.standardError = outputPipe
    outputPipe.fileHandleForReading.readabilityHandler = { [weak self] handle in
      let data = handle.availableData
      if data.isEmpty { return }
      let chunk = String(decoding: data, as: UTF8.self)
      DispatchQueue.main.async {
        self?.appendLog(chunk.trimmingCharacters(in: .newlines))
      }
    }

    proc.terminationHandler = { [weak self] process in
      DispatchQueue.main.async {
        self?.isRunning = false
        self?.process = nil
        self?.statusText = "Service stopped (exit \(process.terminationStatus))"
        self?.appendLog("[info] Service stopped with status \(process.terminationStatus)")
      }
    }

    do {
      try proc.run()
      process = proc
      isRunning = true
      statusText = "Service running"
      appendLog("[info] Started service in \(normalizedPath)")
    } catch {
      statusText = "Failed to start service"
      appendLog("[error] Failed to start: \(error.localizedDescription)")
    }
  }

  func stopService() {
    guard let process else {
      statusText = "Service is not running"
      return
    }

    if process.isRunning {
      process.terminate()
      statusText = "Stopping service"
      appendLog("[info] Stopping service")
    }
  }

  func restartService() {
    if isRunning {
      stopService()
      DispatchQueue.main.asyncAfter(deadline: .now() + 1.0) { [weak self] in
        self?.startService()
      }
      return
    }

    startService()
  }

  func clearLogs() {
    logs = ""
  }

  func updateProjectPath(_ path: String) {
    projectPath = path
  }

  private func currentConfigValues() -> [String: String] {
    [
      "BOT_TOKEN": botToken,
      "ADMIN_USER_IDS": adminUserIds,
      "BOT_TRANSPORT": botTransport.rawValue,
      "DATA_DIR": dataDir,
      "OPENCODE_COMMAND": opencodeCommand,
      "OPENCODE_TIMEOUT_MS": String(opencodeTimeoutMs),
      "PROJECT_PATH": projectPath,
    ]
  }

  private func appendLog(_ line: String) {
    guard !line.isEmpty else { return }
    let timestamp = ISO8601DateFormatter().string(from: Date())
    logs += "[\(timestamp)] \(line)\n"
    if logs.count > 80_000 {
      logs = String(logs.suffix(80_000))
    }
  }

  private static func detectProjectRoot() -> String {
    let cwd = FileManager.default.currentDirectoryPath
    let packageJSON = URL(fileURLWithPath: cwd).appendingPathComponent("package.json").path
    if FileManager.default.fileExists(atPath: packageJSON) {
      return cwd
    }
    return ("~/Documents/Hanamilabs/freelance/opencode-bot" as NSString).expandingTildeInPath
  }
}
