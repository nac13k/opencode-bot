import Foundation

final class ServiceManager: ObservableObject {
  @Published var isRunning = false
  @Published var statusText = "Ready"
  @Published var logs = ""

  @Published var projectPath: String
  @Published var startCommand: String
  @Published var extraEnv: String

  private var process: Process?
  private let storage = UserDefaults.standard

  private enum Keys {
    static let projectPath = "tray.projectPath"
    static let startCommand = "tray.startCommand"
    static let extraEnv = "tray.extraEnv"
  }

  init() {
    let defaultPath = ("~/Documents/Hanamilabs/freelance/opencode-bot" as NSString).expandingTildeInPath
    projectPath = storage.string(forKey: Keys.projectPath) ?? defaultPath
    startCommand = storage.string(forKey: Keys.startCommand) ?? "npm run dev"
    extraEnv = storage.string(forKey: Keys.extraEnv) ?? "NODE_ENV=development\nLOG_LEVEL=debug"
  }

  func saveConfig() {
    storage.set(projectPath, forKey: Keys.projectPath)
    storage.set(startCommand, forKey: Keys.startCommand)
    storage.set(extraEnv, forKey: Keys.extraEnv)
    statusText = "Configuration saved"
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

    saveConfig()

    let proc = Process()
    proc.currentDirectoryURL = URL(fileURLWithPath: normalizedPath)
    proc.executableURL = URL(fileURLWithPath: "/bin/zsh")
    proc.arguments = ["-lc", "exec \(startCommand)"]

    var env = ProcessInfo.processInfo.environment
    parseExtraEnv().forEach { key, value in
      env[key] = value
    }
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

  private func appendLog(_ line: String) {
    guard !line.isEmpty else { return }
    let timestamp = ISO8601DateFormatter().string(from: Date())
    logs += "[\(timestamp)] \(line)\n"
    if logs.count > 80_000 {
      logs = String(logs.suffix(80_000))
    }
  }

  private func parseExtraEnv() -> [String: String] {
    let lines = extraEnv
      .split(separator: "\n")
      .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
      .filter { !$0.isEmpty }

    var result: [String: String] = [:]
    for line in lines {
      let parts = line.split(separator: "=", maxSplits: 1).map(String.init)
      if parts.count == 2 {
        result[parts[0]] = parts[1]
      }
    }
    return result
  }
}
