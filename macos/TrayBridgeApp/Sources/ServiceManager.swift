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
  @Published var allowedUserIds = ""
  @Published var botTransport: BotTransport = .polling
  @Published var dataDir = "./data"
  @Published var opencodeTimeoutMs = 120000
  @Published var opencodeServerUrl = "http://127.0.0.1:4096"
  @Published var opencodeServerUsername = "opencode"
  @Published var opencodeServerPassword = ""
  @Published var defaultSessionId = ""
  @Published var nodeBinaryPath = ""
  @Published var usingBundledServer = false
  @Published var alertMessage: String? = nil
  @Published var pluginStatusText: String? = nil

  private var process: Process?
  private var store: SQLiteConfigStore? = nil

  init() {
    var loadedStore: SQLiteConfigStore?
    do {
      let candidate = try SQLiteConfigStore()
      loadedStore = candidate
      let values = try candidate.loadSettings()
      botToken = values["BOT_TOKEN"] ?? ""
      adminUserIds = values["ADMIN_USER_IDS"] ?? ""
      allowedUserIds = values["ALLOWED_USER_IDS"] ?? ""
      botTransport = BotTransport(rawValue: values["BOT_TRANSPORT"] ?? "polling") ?? .polling
      dataDir = values["DATA_DIR"] ?? "./data"
      opencodeTimeoutMs = Int(values["OPENCODE_TIMEOUT_MS"] ?? "120000") ?? 120000
      opencodeServerUrl = values["OPENCODE_SERVER_URL"] ?? "http://127.0.0.1:4096"
      opencodeServerUsername = values["OPENCODE_SERVER_USERNAME"] ?? "opencode"
      opencodeServerPassword = values["OPENCODE_SERVER_PASSWORD"] ?? ""
      defaultSessionId = values["DEFAULT_SESSION_ID"] ?? ""
      nodeBinaryPath = values["NODE_BINARY"] ?? ""
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

  func installOpenCodePlugin() {
    guard let serverDir = resolveBundledServerDirectory() else {
      pluginStatusText = "Embedded server not found"
      alertMessage = "No se encontro el servidor embebido. Recompila el bundle antes de instalar el plugin."
      appendLog("[error] Plugin install failed: embedded server missing")
      return
    }

    guard let config = buildPluginConfig(serverDir: serverDir) else {
      pluginStatusText = "Plugin config failed"
      appendLog("[error] Plugin install failed: invalid plugin config")
      return
    }

    do {
      let pluginDir = try installPluginFiles(serverDir: serverDir)
      try writePluginConfig(config, pluginDir: pluginDir)
      try registerPlugin(uri: pluginDir.appendingPathComponent("index.ts"))
      pluginStatusText = "Plugin instalado"
      appendLog("[info] OpenCode plugin installed at \(pluginDir.path)")
    } catch {
      pluginStatusText = "Plugin install failed"
      appendLog("[error] Plugin install failed: \(error.localizedDescription)")
    }
  }

  func startService() {
    if isRunning {
      statusText = "Service is already running"
      return
    }

    guard let serverDir = resolveBundledServerDirectory() else {
      usingBundledServer = false
      statusText = "Bundled server not found"
      alertMessage = "La app no incluye el servidor embebido. Recompila con el payload embebido."
      appendLog("[error] App bundle does not include embedded server payload")
      return
    }

    guard let nodePath = resolveNodeBinary() else {
      usingBundledServer = false
      statusText = "Node binary not found"
      alertMessage = "No se encontro Node. Configura NODE_BINARY o instala Node en el sistema."
      appendLog("[error] Node binary not found; set NODE_BINARY or install Node")
      return
    }
    usingBundledServer = true

    let runPath = serverDir.path
    var isDirectory: ObjCBool = false
    if !FileManager.default.fileExists(atPath: runPath, isDirectory: &isDirectory) || !isDirectory.boolValue {
      statusText = "Invalid run path"
      appendLog("[error] Invalid run path: \(runPath)")
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
    proc.currentDirectoryURL = serverDir
    proc.executableURL = URL(fileURLWithPath: "/bin/zsh")
    proc.arguments = ["-lc", "exec \"\(nodePath.path)\" dist/src/main.js"]

    var env = ProcessInfo.processInfo.environment
    env["BOT_TOKEN"] = botToken
    env["ADMIN_USER_IDS"] = adminUserIds
    env["ALLOWED_USER_IDS"] = allowedUserIds
    env["BOT_TRANSPORT"] = botTransport.rawValue
    env["DATA_DIR"] = dataDir
    env["OPENCODE_TIMEOUT_MS"] = String(opencodeTimeoutMs)
    env["OPENCODE_SERVER_URL"] = opencodeServerUrl
    env["OPENCODE_SERVER_USERNAME"] = opencodeServerUsername
    if !opencodeServerPassword.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
      env["OPENCODE_SERVER_PASSWORD"] = opencodeServerPassword
    }
    if !defaultSessionId.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
      env["DEFAULT_SESSION_ID"] = defaultSessionId
    }
    if !nodeBinaryPath.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
      env["NODE_BINARY"] = nodeBinaryPath
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
      appendLog("[info] Started service mode=bundled path=\(runPath)")
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

  private func currentConfigValues() -> [String: String] {
    [
      "BOT_TOKEN": botToken,
      "ADMIN_USER_IDS": adminUserIds,
      "ALLOWED_USER_IDS": allowedUserIds,
      "BOT_TRANSPORT": botTransport.rawValue,
      "DATA_DIR": dataDir,
      "OPENCODE_TIMEOUT_MS": String(opencodeTimeoutMs),
      "OPENCODE_SERVER_URL": opencodeServerUrl,
      "OPENCODE_SERVER_USERNAME": opencodeServerUsername,
      "OPENCODE_SERVER_PASSWORD": opencodeServerPassword,
      "DEFAULT_SESSION_ID": defaultSessionId,
      "NODE_BINARY": nodeBinaryPath,
    ]
  }

  private func resolveBundledServerDirectory() -> URL? {
    guard let resources = Bundle.main.resourceURL else { return nil }
    let server = resources.appendingPathComponent("server", isDirectory: true)
    let mainJs = server.appendingPathComponent("dist/main.js")
    let mainSrcJs = server.appendingPathComponent("dist/src/main.js")
    if FileManager.default.fileExists(atPath: mainJs.path) || FileManager.default.fileExists(atPath: mainSrcJs.path) {
      return server
    }
    return nil
  }

  private func buildPluginConfig(serverDir: URL) -> [String: String]? {
    let resolvedToken = botToken.trimmingCharacters(in: .whitespacesAndNewlines)
    if resolvedToken.isEmpty {
      alertMessage = "BOT_TOKEN es requerido para instalar el plugin."
      return nil
    }

    let dataDirValue = dataDir.trimmingCharacters(in: .whitespacesAndNewlines)
    let resolvedDataDir: String
    if dataDirValue.isEmpty {
      resolvedDataDir = serverDir.appendingPathComponent("data").path
    } else if dataDirValue.hasPrefix("/") {
      resolvedDataDir = dataDirValue
    } else if dataDirValue == "./data" {
      resolvedDataDir = serverDir.appendingPathComponent("data").path
    } else {
      resolvedDataDir = serverDir.appendingPathComponent(dataDirValue).path
    }

    return [
      "dataDir": resolvedDataDir,
      "botToken": resolvedToken,
    ]
  }

  private func installPluginFiles(serverDir: URL) throws -> URL {
    let fm = FileManager.default
    guard let sourceDir = Bundle.main.resourceURL?.appendingPathComponent("server/plugin-global/telegram-relay") else {
      throw PluginInstallError.sourceMissing
    }
    let targetDir = FileManager.default.homeDirectoryForCurrentUser
      .appendingPathComponent(".config/opencode/plugin/telegram-relay", isDirectory: true)

    if fm.fileExists(atPath: targetDir.path) {
      try fm.removeItem(at: targetDir)
    }
    try fm.createDirectory(at: targetDir, withIntermediateDirectories: true)
    try fm.copyItem(at: sourceDir, to: targetDir)
    return targetDir
  }

  private func writePluginConfig(_ config: [String: String], pluginDir: URL) throws {
    let configUrl = pluginDir.appendingPathComponent("config.json")
    let data = try JSONSerialization.data(withJSONObject: config, options: [.prettyPrinted, .sortedKeys])
    try data.write(to: configUrl)
  }

  private func registerPlugin(uri: URL) throws {
    let configDir = FileManager.default.homeDirectoryForCurrentUser
      .appendingPathComponent(".config/opencode", isDirectory: true)
    try FileManager.default.createDirectory(at: configDir, withIntermediateDirectories: true)
    let configUrl = configDir.appendingPathComponent("opencode.json")

    var payload: [String: Any] = [:]
    if let data = try? Data(contentsOf: configUrl), !data.isEmpty {
      if let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any] {
        payload = json
      }
    }

    let pluginUri = "file://" + uri.path
    var plugins = payload["plugin"] as? [String] ?? []
    if !plugins.contains(pluginUri) {
      plugins.append(pluginUri)
    }
    payload["plugin"] = plugins

    let data = try JSONSerialization.data(withJSONObject: payload, options: [.prettyPrinted, .sortedKeys])
    try data.write(to: configUrl)
  }

  private func resolveNodeBinary() -> URL? {
    let trimmed = nodeBinaryPath.trimmingCharacters(in: .whitespacesAndNewlines)
    if !trimmed.isEmpty {
      let explicit = URL(fileURLWithPath: trimmed)
      if FileManager.default.fileExists(atPath: explicit.path) {
        return explicit
      }
      appendLog("[error] NODE_BINARY not found at \(explicit.path)")
    }

    if let whichNode = resolveNodeByWhich() {
      return whichNode
    }

    let candidates = ["/opt/homebrew/bin/node", "/usr/local/bin/node", "/usr/bin/node"]
    for path in candidates {
      if FileManager.default.fileExists(atPath: path) {
        return URL(fileURLWithPath: path)
      }
    }
    return nil
  }

  private func resolveNodeByWhich() -> URL? {
    let proc = Process()
    proc.executableURL = URL(fileURLWithPath: "/usr/bin/which")
    proc.arguments = ["node"]
    let outputPipe = Pipe()
    proc.standardOutput = outputPipe
    proc.standardError = Pipe()
    do {
      try proc.run()
    } catch {
      return nil
    }
    proc.waitUntilExit()
    guard proc.terminationStatus == 0 else { return nil }
    let data = outputPipe.fileHandleForReading.readDataToEndOfFile()
    let path = String(decoding: data, as: UTF8.self).trimmingCharacters(in: .whitespacesAndNewlines)
    if path.isEmpty { return nil }
    return URL(fileURLWithPath: path)
  }

  private func appendLog(_ line: String) {
    guard !line.isEmpty else { return }
    let timestamp = ISO8601DateFormatter().string(from: Date())
    logs += "[\(timestamp)] \(line)\n"
    if logs.count > 80_000 {
      logs = String(logs.suffix(80_000))
    }
    if line.contains("getMe") && line.contains("404") {
      alertMessage = "BOT_TOKEN invalido. Debe tener formato <numero>:<texto> (ej: 123456789:AAAbbbCCC)."
    }
  }

  private enum PluginInstallError: Error {
    case sourceMissing
  }
}
